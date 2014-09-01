package main

import (
	"flag"
	"go/build"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	version = "0.1.8"
)

func main() {
	log.Printf("Livedev %s\n", version)

	configFile := flag.String("c", "", "Configuration file")
	log.SetOutput(os.Stderr)
	flag.Parse()

	if len(*configFile) == 0 {
		flag.Usage()
		return
	}

	conf, err := LoadConfig(*configFile)

	if err != nil {
		log.Fatal(err)
	}

	if conf.Port == 0 {
		conf.Port = 80
	}

	if s := strings.TrimSpace(conf.GOROOT); len(s) > 0 {
		if err := os.Setenv(envGoroot, s); err != nil {
			log.Fatal(err)
		}
	}

	if len(conf.GOPATH) > 0 {
		p := strings.Join(conf.GOPATH, string(filepath.ListSeparator))
		if err := os.Setenv(envGopath, p); err != nil {
			log.Fatal(err)
		}
	}

	var (
		GOPATH        = filepath.SplitList(os.Getenv(envGopath))
		GOROOT        = os.Getenv(envGoroot)
		servers       = make(map[string]*Server)
		defaultServer *Server
	)

	for _, s := range conf.Server {
		context := build.Default
		s.GOROOT = strings.TrimSpace(s.GOROOT)

		if len(s.GOROOT) == 0 {
			s.GOROOT = GOROOT
		}

		if len(s.GOPATH) == 0 {
			s.GOPATH = GOPATH
		}

		s.Workspace = strings.TrimSpace(s.Workspace)

		if len(s.Workspace) > 0 {
			s.GOPATH = append(s.GOPATH, s.Workspace)
		}

		context.GOROOT = s.GOROOT

		context.GOPATH = strings.Join(s.GOPATH, string(filepath.ListSeparator))
		s.Host = strings.TrimSpace(s.Host)

		if len(s.Host) == 0 {
			s.Host = "localhost"
		}

		if _, dup := servers[s.Host]; dup {
			log.Fatalf(`Fatal error: Duplicate server name "%s"`, s.Host)
		}

		srv, err := NewServer(context, s)

		if err != nil {
			log.Fatalf(`Fatal error: Server binary not found "%s" : %v`, s.Host, err)
		}

		servers[s.Host] = srv
		log.Printf("Host: %s\n", net.JoinHostPort(srv.host, strconv.Itoa(srv.port)))

		if defaultServer == nil || s.Default {
			defaultServer = srv
		}
	}

	p := NewProxy(conf.Port, servers, defaultServer)
	log.Printf("Proxy: %s\n", net.JoinHostPort("localhost", strconv.Itoa(conf.Port)))

	if err := p.ListenAndServe(); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
}
