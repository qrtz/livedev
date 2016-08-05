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

	"github.com/qrtz/livedev/watcher"
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

	conf := config{
		Port:           80,
		GoRoot:         os.Getenv(envGoroot),
		GoPath:         filepath.SplitList(os.Getenv(envGopath)),
		StartupTimeout: 10, // Default startup timeout in seconds
	}

	if err := loadConfig(*configFile, &conf); err != nil {
		log.Fatal(err)
	}

	if err := os.Setenv(envGoroot, conf.GoRoot); err != nil {
		log.Fatal(err)
	}

	if err := os.Setenv(envGopath, strings.Join(conf.GoPath, string(filepath.ListSeparator))); err != nil {
		log.Fatal(err)
	}
	w, err := watcher.New()

	if err != nil {
		log.Fatal(err)
	}

	defer w.Close()

	var (
		servers       = make(map[string]*Server)
		defaultServer *Server
	)

	for _, s := range conf.Servers {
		context := build.Default

		context.GOROOT = s.GoRoot

		context.GOPATH = strings.Join(s.GoPath, string(filepath.ListSeparator))

		if _, dup := servers[s.Host]; dup {
			log.Fatalf(`Fatal error: Duplicate server name "%s"`, s.Host)
		}

		srv, err := newServer(context, s, conf.Port, w)

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
