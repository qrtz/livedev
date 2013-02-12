package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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

type Server struct {
	tcpAddr   *net.TCPAddr
	startTime time.Time
	bin       string
	target    string
	cmd       *exec.Cmd
	lock      sync.Mutex
	startup   []string
	port      int
	portType  PortType
	host      string
	source    []string
	builder   *Builder
	addr      string
	skip      *regexp.Regexp
	closed    chan struct{}
	state     error
}

func NewServer(bin string) (*Server, error) {
	p := new(Server)
	p.bin = bin
	return p, nil
}

func (srv *Server) wait(timeout time.Duration) error {
	var (
		c         = time.After(timeout)
		lastError error
	)
	url := fmt.Sprintf("http://%s/", srv.addr)

	for {
		select {
		case <-c:
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
		var lines []string
		if len(out) > 0 {
			r := bufio.NewReader(bytes.NewReader(out))
		done:
			for {
				line, err := r.ReadString('\n')
				switch {
				case err != nil:
					break done
				default:
					lines = append(lines, line)
				}
			}
			log.Println(strings.Join(lines, "\n"))
		}
		if len(lines) > 0 {
			srv.state = errors.New(strings.Join(lines, "\n"))
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
	return srv.wait(30 * time.Second)
}

func (srv *Server) build() error {
	log.Printf("Building...%s", srv.host)
	return srv.builder.Build(srv.target, srv.source, srv.bin)
}

func (srv *Server) BuildAndRun() error {
	srv.lock.Lock()
	defer srv.lock.Unlock()

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
		return E_PROXY_ONLY
	}

	var (
		binModTime,
		buildFileTime,
		appFileTime time.Time
	)

	if stat, err := os.Stat(srv.bin); err == nil {
		binModTime = stat.ModTime()
	}

	if len(srv.source) > 0 {
		bt, at, err := ModTimeList(srv.source, srv.skip)
		if err != nil {
			return err
		}

		buildFileTime, appFileTime = bt, at
	}

	rebuild := len(srv.target) > 0 && buildFileTime.After(binModTime)
	restart := rebuild || srv.cmd == nil || binModTime.After(srv.startTime) || appFileTime.After(srv.startTime)

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
