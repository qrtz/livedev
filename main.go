package main


import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func Usage(name string) {
	fmt.Printf(`Usage: %s [options]
	`, name)
}

func init() {
	flag.StringVar(&config, "c", "", "configuration file")
	log.SetOutput(os.Stderr)
}

func main() {
	flag.Parse()

	if len(config) == 0 {
		flag.Usage()
		return
	}

	conf, err := LoadConfig(config)

	if err != nil {
		log.Fatal(err)
	}

	if conf.Port == 0 {
		conf.Port = 80
	}

	if s := strings.TrimSpace(conf.GOROOT); len(s) > 0 {
		if err := os.Setenv(KEY_GOROOT, s); err != nil {
			log.Fatal(err)
		}
	}

	if s := strings.TrimSpace(conf.GOPATH); len(s) > 0 {
		if err := os.Setenv(KEY_GOPATH, s); err != nil {
			log.Fatal(err)
		}
	}

	GOROOT = os.Getenv(KEY_GOROOT)
	GOPATH = os.Getenv(KEY_GOPATH)

	if len(GOROOT) > 0 {
		GOBIN = filepath.Join(GOROOT, "bin", "go")
	} else {
		GOBIN = "go"
	}

	builder := NewBuilder(GOBIN)

	servers := make(map[string]*Server)

	var defaultServer *Server

	for _, s := range conf.Server {

		h := strings.TrimSpace(s.Host)

		if len(h) == 0 {
			h = "localhost"
		}

		s.Host = h

		if _, dup := servers[s.Host]; dup {
			log.Fatalf(`Fatal error: Duplicate server name "%s"`, s.Host)
		}

		srv, err := NewServer(strings.TrimSpace(s.Bin))

		if err != nil {
			log.Fatalf(`Fatal error: Server binary not found "%s" : %v`, s.Host, err)
		}

		if pattern := strings.TrimSpace(s.Skip); len(pattern) > 0 {
			p, err := regexp.Compile(pattern)
			if err != nil {
				log.Fatalf(`Fatal error: Invalid pattern "%s" : %v`, pattern, err)
			}
			srv.skip = p
		}

		for _, x := range s.Source {
			if p := strings.TrimSpace(x); len(p) > 0 {
				srv.source = append(srv.source, strings.TrimSpace(x))
			}
		}

		srv.builder = builder
		srv.target = strings.TrimSpace(s.Target)
		srv.port = s.Port
		srv.startup = s.Startup
		srv.host = strings.TrimSpace(s.Host)
		servers[s.Host] = srv
		log.Printf("Host: %s\n", net.JoinHostPort(srv.host, strconv.Itoa(srv.port)))

		if defaultServer == nil || s.Default {
			defaultServer = srv
		}
	}

	proxyAddr := net.JoinHostPort("localhost", strconv.Itoa(conf.Port))
	p := NewProxy(conf.Port, servers, defaultServer)
	log.Printf("Proxy: %s\n", proxyAddr)


	if err := p.ListenAndServe(); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}

	log.Println("Server stopped")
}
