package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
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
	srv, found := p.servers[r.Host]
	defer r.Body.Close()

	if !found {
		log.Printf("Host (%s:%v) not found. Reverting to default\n", r.Host, r.URL.String())
		srv = p.defaultServer
	}

	if srv == nil {
		http.Error(w, fmt.Sprintf(`Host not found "%s"`, r.Host), http.StatusNotFound)
		return
	}

	if err := srv.BuildAndRun(); err != nil {
		http.Error(w, fmt.Sprintf("BUILD ERROR: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if p.addr.Port == srv.port && srv.tcpAddr.IP.IsLoopback() {
		http.Error(w, fmt.Sprintf(`Invalid host configuration "%s"`, r.Host), http.StatusNotFound)
		return
	}

	if err := srv.ServeHTTP(w, r); err != nil {
		http.Error(w, fmt.Sprintf("SERVER ERROR: %s", err.Error()), http.StatusInternalServerError)
	}

}

func (p *Proxy) ListenAndServe() error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p.ServeHTTP(w, r)
	})

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", strconv.Itoa(p.port)))
	if err != nil {
		return err
	}

	p.addr = addr

	return http.ListenAndServe(p.addr.String(), nil)
}
