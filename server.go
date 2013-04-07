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
	E_TIMEOUT    = errors.New("Timeout:Gave up")
	E_PROXY_ONLY = errors.New("proxy only")
)

type PortType uint8

const (
	PROT_TYPE_STATIC = iota
	PORT_TYPE_DYNAMIC
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
	portType    PortType
	requestTime time.Time
	resources   *Resource
	startup     []string
	startTime   time.Time
	state       error
	target      string
	tcpAddr     *net.TCPAddr
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
	srv.bin = strings.TrimSpace(s.Bin)
	srv.builder = s.Builder

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
		srv.builder = append(srv.builder, cmd, "build", "-o", srv.bin, srv.target)
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
	url := fmt.Sprintf("http://%s/", srv.addr)

	for {
		select {
		case <-timeout:
			return E_TIMEOUT
		default:
			if srv.state != nil {
				return srv.state
			}
			response, err := http.Head(url)

			if err == nil {
				if response != nil && response.Body != nil {
					response.Body.Close()
				}
				return nil
			}

			lastError = err
			time.Sleep(100 * time.Millisecond)
		}
	}

	return lastError
}

func (srv *Server) Stop() (err error) {
	if srv.cmd != nil && srv.cmd.Process != nil {
		err = srv.cmd.Process.Kill()
		<-srv.closed
	}
	return
}

func (srv *Server) monitor() {
	defer close(srv.closed)
	if out, err := srv.cmd.CombinedOutput(); err != nil {
		if len(out) > 0 {
			srv.state = errors.New(string(out))
		} else {
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
	args := srv.startup
	if srv.port == 0 {
		addr, err := findAvailablePort()

		if err != nil {
			return err
		}
		srv.tcpAddr = addr
		srv.port = addr.Port
		srv.portType = PORT_TYPE_DYNAMIC
	}

	if len(srv.addr) == 0 {
		srv.addr = net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
		log.Println(srv.addr)
	}

	if srv.tcpAddr == nil {
		a, err := net.ResolveTCPAddr("tcp", srv.addr)
		if err != nil {
			return err
		}
		srv.tcpAddr = a
	}

	if srv.portType == PORT_TYPE_DYNAMIC {
		args = append(args, "--addr", srv.addr)
	}

	srv.state = nil
	srv.cmd = exec.Command(srv.bin, args...)
	srv.startTime = time.Now()
	srv.closed = make(chan struct{})
	go srv.monitor()
	return srv.wait(time.After(30 * time.Second))
}

func (srv *Server) build() error {
	log.Printf("Building...%s", srv.host)
	//Reset the dependency list.
	srv.dep = srv.dep[0:0]

	env := NewEnv(os.Environ())
	env.Set(KEY_GOPATH, srv.context.GOPATH)

	var (
		command string
		args    []string
	)

	command, args = srv.builder[0], srv.builder[1:]

	cmd := exec.Command(command, args...)
	cmd.Env = env.Data()

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

	return nil

}

func (srv *Server) BuildAndRun() error {
	requestTime := time.Now()
	srv.lock.Lock()

	defer func() {
		srv.requestTime = time.Now()
		srv.lock.Unlock()
	}()

	if len(srv.bin) == 0 {
		if srv.port == 0 {
			srv.port = 80
		}
		if len(srv.addr) == 0 {
			srv.addr = net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
			a, err := net.ResolveTCPAddr("tcp", srv.addr)
			if err != nil {
				return err
			}
			srv.tcpAddr = a

		}
		//PROXY ONLY
		return nil
	}

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

	if stat, err := os.Stat(srv.bin); err == nil {
		binModTime = stat.ModTime()
		if !restart {
			restart = binModTime.After(srv.startTime)
		}
	}

	if len(srv.target) > 0 && len(srv.dep) == 0 {
		dep, err := ComputeDep(&srv.context, srv.target)
		if err != nil {
			return err
		}
		srv.dep = dep
	}

	mtime, err := ModTimeList(srv.dep, nil)

	if err != nil {
		return err
	}

	rebuild = mtime.After(binModTime)
	restart = restart || rebuild

	if !restart && srv.resources != nil {
		mtime, err := ModTimeList(srv.resources.Paths, srv.resources.Ignore)

		if err != nil {
			return err
		}
		restart = mtime.After(srv.startTime)
	}

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

	log.Println(r.RemoteAddr, r.URL, srv.host)
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
