package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	version = "0.2.1"
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

	conf, err := loadConfig(*configFile)

	if err != nil {
		log.Fatal(err)
	}

	if conf.Port == 0 {
		conf.Port = 80
	}

	if s := strings.TrimSpace(conf.GoRoot); len(s) > 0 {
		fmt.Println("GOROOT", conf.GoRoot)
		if err := os.Setenv(envGoroot, s); err != nil {
			log.Fatal(err)
		}
	} else {
		conf.GoRoot = os.Getenv(envGoroot)
	}

	if len(conf.GoPath) > 0 {
		p := strings.Join(conf.GoPath, string(filepath.ListSeparator))
		if err := os.Setenv(envGopath, p); err != nil {
			log.Fatal(err)
		}
	} else {
		conf.GoPath = filepath.SplitList(os.Getenv(envGopath))
	}

	var (
		servers       = make(map[string]*Server)
		defaultServer *Server
	)

	for _, s := range conf.Servers {
		context := build.Default
		s.GoRoot = strings.TrimSpace(s.GoRoot)

		if len(s.GoRoot) == 0 {
			s.GoRoot = conf.GoRoot
		}

		if len(s.GoPath) == 0 {
			s.GoPath = conf.GoPath
		}

		s.Workspace = strings.TrimSpace(s.Workspace)

		if len(s.Workspace) > 0 {
			s.GoPath = append(s.GoPath, s.Workspace)
		}

		context.GOROOT = s.GoRoot

		context.GOPATH = strings.Join(s.GoPath, string(filepath.ListSeparator))
		s.Host = strings.TrimSpace(s.Host)

		if len(s.Host) == 0 {
			s.Host = "localhost"
		}

		if _, dup := servers[s.Host]; dup {
			log.Fatalf(`Fatal error: Duplicate server name "%s"`, s.Host)
		}

		srv, err := newServer(context, s)

		if err != nil {
			log.Fatalf(`Fatal error: Server binary not found "%s" : %v`, s.Host, err)
		}

		servers[s.Host] = srv
		log.Printf("Host: %s\n", net.JoinHostPort(srv.host, strconv.Itoa(srv.port)))

		if defaultServer == nil || s.Default {
			defaultServer = srv
		}
	}

	p := newProxy(conf.Port, servers, defaultServer)
	log.Printf("Proxy: %s\n", net.JoinHostPort("localhost", strconv.Itoa(conf.Port)))

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, os.Kill)
	go func() {
		<-exit
		p.shutdown()
		os.Exit(0)
	}()

	fmt.Println("Exit Status: ", p.ListenAndServe())
}
