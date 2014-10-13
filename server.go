package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	errTimeout = errors.New("Timeout:Gaving up")
)

type Resource struct {
	Ignore *regexp.Regexp
	Paths  []string
}

type Server struct {
	addr        string
	bin         string
	builder     []string
	closed      chan struct{}
	cmd         *exec.Cmd
	context     build.Context
	dep         []string
	host        string
	lock        sync.Mutex
	port        int
	requestTime time.Time
	resources   *Resource
	startup     []string
	startTime   time.Time
	state       error
	target      string
	targetDir   string
}

func NewServer(context build.Context, s ServerConf) (*Server, error) {
	var (
		srv    = new(Server)
		ignore *regexp.Regexp
		paths  []string
	)

	if len(s.Resources.Paths) > 0 {
		for _, s := range s.Resources.Paths {
			if p := strings.TrimSpace(s); len(p) > 0 {
				paths = append(paths, p)
			}
		}

		if len(paths) > 0 {
			if s := strings.TrimSpace(s.Resources.Ignore); len(s) > 0 {
				if pattern, err := regexp.Compile(s); err == nil {
					ignore = pattern
				} else {
					return nil, fmt.Errorf(`Fatal error: Invalid pattern "%s" : %v`, s, err)
				}
			}

			srv.resources = &Resource{ignore, paths}
		}
	}

	srv.target = strings.TrimSpace(s.Target)
	srv.targetDir = filepath.Dir(s.Target)
	srv.bin = strings.TrimSpace(s.Bin)
	srv.builder = s.Builder

	if len(srv.bin) == 0 {
		srv.bin = filepath.Join(os.TempDir(), "livedev-"+s.Host)
	}

	if !hasPrefix(srv.target, filepath.SplitList(context.GOPATH)) {
		//Target is not in the $GOPATH
		//Try to guess the import root(workspace) from the path
		roots := ImportRoots(srv.target)

		if len(roots) > 0 {
			context.GOPATH = strings.Join(append(roots, context.GOPATH), string(filepath.ListSeparator))
		}
	}

	srv.context = context

	if len(srv.builder) == 0 {

		cmd := filepath.Join(context.GOROOT, "bin", "go")
		srv.builder = append(srv.builder, cmd, "build", "-o", srv.bin)
	}

	srv.port = s.Port
	srv.startup = s.Startup
	srv.host = s.Host
	return srv, nil
}

func (srv *Server) wait(timeout <-chan time.Time) error {
	var (
		lastError error
	)
	u := fmt.Sprintf("http://%s/", srv.addr)

	for {
		select {
		case <-timeout:
			return errTimeout
		default:
			if srv.state != nil {
				return srv.state
			}
			response, err := http.Head(u)

			if err == nil {
				if response != nil && response.Body != nil {
					response.Body.Close()
				}

				log.Printf("Started: %s\n", srv.addr)
				return nil
			}

			lastError = err
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("Unable to start: %s\n", srv.addr)

	return lastError
}

func (srv *Server) Stop() (err error) {
	log.Printf("Stopping...%s:%v", srv.host, srv.port)
	if srv.cmd != nil && srv.cmd.Process != nil {
		err = srv.cmd.Process.Kill()
		if err == nil {
			err = srv.cmd.Process.Release()
		}
		<-srv.closed
	}

	if err == nil {
		srv.state, err = nil, srv.state
	}

	return err
}

func (srv *Server) monitor() {
	defer func() {
		select {
		case <-srv.closed:
			return
		default:
			close(srv.closed)
		}
	}()

	var stderr bytes.Buffer
	srv.cmd.Stderr = &stderr

	if err := srv.cmd.Start(); err != nil {
		srv.state = err
		srv.cmd = nil
		return
	}

	go func() {
		for {
			select {
			case <-srv.closed:
				return
			default:
				var buf [2 << 10]byte
				if ln, _ := stderr.Read(buf[:]); ln > 0 {
					srv.state = errors.New(string(buf[:ln]))
				}
			}
		}
	}()

	if err := srv.cmd.Wait(); err != nil {
		close(srv.closed)
		if srv.state == nil {
			srv.state = err
		}
	}

	srv.cmd = nil
}

func (srv *Server) Start() error {

	if srv.cmd != nil {
		log.Printf(`"%s" already started`, srv.host)
		return nil
	}

	log.Printf("Starting...%s", srv.host)

	generate_port := srv.port == 0

	if generate_port {
		addr, err := findAvailablePort()

		if err != nil {
			return err
		}
		srv.port = addr.Port
	}

	if len(srv.addr) == 0 {
		srv.addr = net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
		log.Println(srv.addr)
	}

	if _, err := net.ResolveTCPAddr("tcp", srv.addr); err != nil {
		return err
	}

	// The verser must accept "--addr" argument if no port is specified in the configuration
	if generate_port {
		srv.startup = append(srv.startup, "--addr", srv.addr)
	}

	srv.state = nil
	srv.cmd = exec.Command(srv.bin, srv.startup...)
	srv.startTime = time.Now()
	srv.closed = make(chan struct{})
	go srv.monitor()
	log.Printf("Waiting for ......%s", srv.host)
	return srv.wait(time.After(30 * time.Second))
}

func (srv *Server) build() error {
	log.Printf("Building...%s", srv.host)

	//List of file to pass to "go build"
	var buildFiles []string

	for _, f := range srv.dep {
		if strings.HasPrefix(f, srv.targetDir) {
			buildFiles = append(buildFiles, filepath.Base(f))
		}
	}

	//Reset the dependency list.

	env := NewEnv(os.Environ())
	env.Set(envGopath, srv.context.GOPATH)

	command, args := srv.builder[0], srv.builder[1:]

	args = append(args, buildFiles...)
	cmd := exec.Command(command, args...)
	cmd.Env = env.Data()
	cmd.Dir = srv.targetDir

	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) > 0 {
			r := bufio.NewReader(bytes.NewReader(out))
			var lines []string
		done:
			for {
				line, err := r.ReadString('\n')
				switch {
				case err != nil:
					break done
				case !strings.HasPrefix(line, "#"):
					lines = append(lines, line)
				}
			}
			return errors.New(strings.Join(lines, ""))
		}

		return err
	}

	srv.dep = nil
	return nil

}

func (srv *Server) BuildAndRun() error {
	requestTime := time.Now()
	srv.lock.Lock()

	defer func() {
		srv.requestTime = time.Now()
		srv.lock.Unlock()
	}()

	//Ignore if the request is within less than 3 seconds of the last one.
	//This is an arbitrary number. We can certainly increase this.
	if requestTime.Sub(srv.requestTime) < (3 * time.Second) {
		return nil
	}

	var (
		binModTime time.Time
		restart    = srv.cmd == nil
		rebuild    bool
	)

	if len(srv.bin) == 0 {
		rebuild = true
		srv.bin = filepath.Join(os.TempDir(), "livedev-"+srv.host)

	} else {
		stat, err := os.Stat(srv.bin)

		if err != nil && !os.IsNotExist(err) {
			return err
		}

		if err == nil {
			if stat.Size() > 0 {
				binModTime = stat.ModTime()
				restart = restart || binModTime.After(srv.startTime)
			}
		}
	}

	if len(srv.target) > 0 && len(srv.dep) == 0 {
		dep, err := ComputeDep(&srv.context, srv.target)
		if err != nil {
			return err
		}
		srv.dep = dep
	}

	var err error
	rebuild, err = ModifiedSince(binModTime, nil, srv.dep...)
	if err != nil {
		return err
	}

	restart = restart || rebuild

	if !restart && srv.resources != nil {
		restart, err = ModifiedSince(srv.startTime, srv.resources.Ignore, srv.resources.Paths...)
		if err != nil {
			return err
		}
	}

	log.Printf("\nREBUILD: %v | RESTART: %v\n", rebuild, restart)

	if !restart {
		return nil
	}

	if err := srv.Stop(); err != nil {
		return err
	}

	if rebuild {
		if err := srv.build(); err != nil {
			return err
		}
	}

	return srv.Start()
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	transport := new(http.Transport)
	h, ok := w.(http.Hijacker)

	if !ok {
		return errors.New("Unable to hijack connection")
	}

	r.Host = srv.addr
	r.URL.Host = r.Host

	if len(r.URL.Scheme) == 0 {
		r.URL.Scheme = "http"
	}

	response, err := transport.RoundTrip(r)

	if err != nil {
		return err
	}

	conn, _, err := h.Hijack()

	if err != nil {
		return err
	}

	defer conn.Close()
	defer response.Body.Close()
	return response.Write(conn)
}
