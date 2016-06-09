package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/qrtz/livedev/gosource"
)

type proxy struct {
	addr          *net.TCPAddr
	port          int
	servers       map[string]*Server
	defaultServer *Server
	codeViewerMux *serveMux
}

type serveMux struct {
	Handler *http.ServeMux
	Addr    string
	Port    int
}

func newProxy(port int, servers map[string]*Server, defaultServer *Server) *proxy {
	p := &proxy{
		port:          port,
		servers:       servers,
		defaultServer: defaultServer,
	}
	p.codeViewerMux = codeViewer(p.servers)
	return p
}

type StackFrame struct {
	Line       int64
	Text, File string
}

type ServerError struct {
	Message string
	Name    string
	Data    []Node
}

func resolvePath(f string, dirs []string) (dir, path string) {
	isAbs := filepath.IsAbs(f)
	for _, dir := range dirs {
		if isAbs {
			if strings.HasPrefix(f, dir) {
				return dir, strings.TrimPrefix(f, dir)[1:]
			}
		} else if p := filepath.Join(dir, f); fileExists(p) {
			return dir, strings.TrimPrefix(p, dir)[1:]
		}
	}
	return dir, path
}

type Node struct {
	Text, Link, Line string
}

func parseError(gopaths []string, prefix string, err []byte) (lines []Node) {
	var b []byte
	var ln []byte
	var filename string
	for _, c := range err {
		switch c {
		case ':':
			if len(ln) == 0 {
				s := strings.TrimSpace(string(b))
				if dir, f := resolvePath(s, gopaths); len(dir) > 0 {
					filename = filepath.Join(prefix, f)
				}
			}
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if len(filename) > 0 {
				ln = append(ln, c)
			}
		case '\n':
			lines = append(lines, Node{Text: string(append(b, c))})
			b = b[:0]
		default:
			if len(filename) > 0 {
				lines = append(lines, Node{Text: string(b), Link: filename, Line: string(ln)})
				b = b[:0]
				filename = filename[:0]
				ln = ln[:0]
			}
		}

		if c != '\n' {
			b = append(b, c)
		}
	}

	if len(b) > 0 {
		lines = append(lines, Node{Text: string(b)})
	}

	return lines
}

func LastIndexOf(s string, b byte) int {
	for i := len(s) - 1; i > 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func SplitPathLine(path string) (string, int64, error) {
	if i := LastIndexOf(path, ':'); i >= 0 {
		line, err := strconv.ParseInt(path[i+1:], 10, 32)
		return path[:i], line, err
	}
	return path, -1, errors.New("No line number")
}

func codeViewer(servers map[string]*Server) *serveMux {
	managerMux := &serveMux{Handler: http.NewServeMux()}
	managerMux.Handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Title     string
			Lines     []*gosource.Line
			ErrorLine int64
		}
		if len(r.URL.Path) == 1 {
			http.NotFound(w, r)
			return
		}

		hostname, _, _ := net.SplitHostPort(r.Host)

		srv, ok := servers[hostname]

		if !ok {
			http.Error(w, "Server not found: "+hostname, http.StatusNotFound)
			return
		}

		path, line, err := SplitPathLine(r.URL.Path[1:])

		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		data.ErrorLine = line
		srcDirs := append(srv.context.SrcDirs(), srv.targetDir)

		if dir, path := resolvePath(path, srcDirs); len(dir) > 0 {
			filename := filepath.Join(dir, path)
			lines, err := gosource.Parse(filename)
			if err == nil {
				data.Lines = lines
			}
			data.Title = "Source: " + path
		}

		if len(data.Lines) > 0 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			codeviewerTemplate.Execute(w, data)
		} else {
			http.Error(w, "File not found: "+path, http.StatusNotFound)
		}
	})

	return managerMux
}

func (p *proxy) handleError(w http.ResponseWriter, err ServerError, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	errTemplate.Execute(w, err)
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		srv  *Server
		host = r.Host
	)

	defer func() {
		if err := recover(); err != nil {
			var buf [2 << 10]byte
			errData := ServerError{Name: "Unknown Error"}

			if srv != nil {
				addr := net.JoinHostPort(srv.host, strconv.Itoa(p.codeViewerMux.Port)) + "/"
				errData.Data = parseError(append(srv.context.SrcDirs(), srv.targetDir), addr, buf[:runtime.Stack(buf[:], false)])
			}
			p.handleError(w, errData, http.StatusInternalServerError)
		}
	}()

	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}

	srv = p.servers[host]

	if srv == nil {
		if p.defaultServer != nil {
			srv = p.defaultServer
		} else {
			http.Error(w, fmt.Sprintf(`Host not found "%s"`, host), http.StatusNotFound)
			return
		}
	}

	if err := srv.ServeHTTP(w, r); err != nil {
		if r.Header.Get("Upgrade") == "websocket" {
			conn, buf, err := w.(http.Hijacker).Hijack()
			writeWebSocketError(buf, err, http.StatusInternalServerError)
			conn.Close()
		} else {
			errData := ServerError{Name: "Error"}
			errData.Data = parseError(append(srv.context.SrcDirs(), srv.targetDir), net.JoinHostPort(srv.host, strconv.Itoa(p.codeViewerMux.Port)), []byte(err.Error()))
			p.handleError(w, errData, http.StatusInternalServerError)
		}
	}
}

func (p *proxy) ListenAndServe() error {
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", strconv.Itoa(p.port)))
	if err != nil {
		return err
	}

	p.addr = addr
	done := make(chan error, 1)
	go func() {
		done <- http.ListenAndServe(p.addr.String(), p)
	}()

	select {
	case err := <-done:
		done <- err
	default:
		if addr, err := findAvailablePort(); err == nil {
			go func(port int) {
				p.codeViewerMux.Port = port
				p.codeViewerMux.Addr = net.JoinHostPort("", strconv.Itoa(port))
				done <- http.ListenAndServe(p.codeViewerMux.Addr, p.codeViewerMux.Handler)
			}(addr.Port)
		} else {
			done <- err
		}
	}

	err = <-done

	for _, s := range p.servers {
		go s.Shutdown()
	}

	return err
}
