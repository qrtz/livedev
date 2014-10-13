package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
)

type Proxy struct {
	addr          *net.TCPAddr
	port          int
	servers       map[string]*Server
	defaultServer *Server
}

func NewProxy(port int, servers map[string]*Server, defaultServer *Server) *Proxy {
	return &Proxy{
		port:          port,
		servers:       servers,
		defaultServer: defaultServer,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	srv, found := p.servers[host]
	defer r.Body.Close()

	defer func() {
		if err := recover(); err != nil {
			var buf [2 << 10]byte
			http.Error(w, fmt.Sprintf("Unknown Error: %v \nTrace: \n%s", err, buf[:runtime.Stack(buf[:], false)]), http.StatusInternalServerError)
		}
	}()

	if !found {
		log.Printf("Host (%s:%v) not found. Reverting to default\n", host, r.URL.String())
		srv = p.defaultServer
	}

	if srv == nil {
		http.Error(w, fmt.Sprintf(`Host not found "%s"`, host), http.StatusNotFound)
		return
	}

	if err := srv.BuildAndRun(); err != nil {
		http.Error(w, fmt.Sprintf("BUILD ERROR: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if err := srv.ServeHTTP(w, r); err != nil {
		if srv.state != nil {
			srv.state, err = nil, srv.state
		}

		http.Error(w, fmt.Sprintf("Runtime Error: %s", err.Error()), http.StatusInternalServerError)
	}
}

func (p *Proxy) ListenAndServe() error {

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", strconv.Itoa(p.port)))
	if err != nil {
		return err
	}

	p.addr = addr

	return http.ListenAndServe(p.addr.String(), p)
}
