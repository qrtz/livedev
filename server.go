package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/build"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"compress/gzip"
	"io/ioutil"

	"github.com/qrtz/livedev/env"
	"github.com/qrtz/livedev/logger"
	"github.com/qrtz/livedev/watcher"
)

var (
	errTimeout            = errors.New("Timeout:Gaving up")
	errInvalidFilePattern = errors.New("Invalid file pattern")
)

type processState uint32

const (
	created processState = iota
	running
	stopping
	exited
)

type resource struct {
	Ignore *regexp.Regexp
	Paths  map[string]struct{}
}

func newResource(paths []string, ignore string) (*resource, error) {

	rs := &resource{Paths: make(map[string]struct{})}

	if len(paths) > 0 {
		for _, s := range paths {
			if p := strings.TrimSpace(s); len(p) > 0 {
				rs.Paths[filepath.Clean(p)] = struct{}{}
			}
		}

		if s := strings.TrimSpace(ignore); len(s) > 0 {
			pattern, err := regexp.Compile(s)
			if err != nil {
				return nil, err
			}
			rs.Ignore = pattern
		}
	}

	return rs, nil
}

// Watch adds Resource files and directories to the given watcher
func (r resource) Walk(walkFunc func(string) error) {
	for f := range r.Paths {
		if info, err := os.Lstat(f); err == nil {
			if info.IsDir() {
				filepath.Walk(f, func(path string, info os.FileInfo, err error) error {
					if err == nil && info.IsDir() {
						if r.Ignore != nil && r.Ignore.MatchString(path) {
							return filepath.SkipDir
						}
						return walkFunc(path)

					}
					return nil
				})
			} else {
				walkFunc(f)
			}
		}
	}
}

// MathcPath tests whehere the given string matches any of the resource files
func (r resource) MatchPath(p string) bool {
	for f := range r.Paths {
		if strings.HasPrefix(p, f) {
			return r.Ignore == nil || !r.Ignore.MatchString(p)
		}
	}
	return false
}

type updateListeners struct {
	mu        sync.Mutex
	listeners map[chan struct{}]struct{}
}

func newUpdateListeners() *updateListeners {
	return &updateListeners{
		listeners: make(map[chan struct{}]struct{}),
	}
}

func (u *updateListeners) register() <-chan struct{} {
	u.mu.Lock()
	defer u.mu.Unlock()

	ch := make(chan struct{})
	u.listeners[ch] = struct{}{}
	return ch
}

func (u *updateListeners) remove(ch chan struct{}) {
	u.mu.Lock()
	defer u.mu.Unlock()

	delete(u.listeners, ch)
	close(ch)
}

func (u *updateListeners) notify() {
	u.mu.Lock()
	defer u.mu.Unlock()
	for ch := range u.listeners {
		select {
		case ch <- struct{}{}:
		default:
			go u.remove(ch)
		}
	}
}

// Server represents an http server
type Server struct {
	addr           string
	bin            string
	builder        []string
	closed         chan struct{}
	context        build.Context
	dep            map[string]struct{}
	host           string
	port           int
	resources      *resource
	assets         *resource
	startup        []string
	target         string
	targetDir      string
	stdout         *logger.LogWriter
	stderr         *logger.BufferedLogWriter
	startupTimeout time.Duration
	watcher        *watcher.Watcher
	watcherEvents  chan watcher.Event
	pending        sync.WaitGroup

	updateListeners *updateListeners

	busy    chan bool
	ready   chan error
	stopped chan bool
	started chan bool
	exit    chan bool
	done    chan error

	mu    sync.Mutex
	error error

	once sync.Once

	cmd          *exec.Cmd
	processState uint32
	conf         serverConfig
	proxyPort    int
}

func (srv *Server) setProcessState(state processState) {
	atomic.StoreUint32(&srv.processState, uint32(state))
}

func (srv *Server) getProcessState() processState {
	return processState(atomic.LoadUint32(&srv.processState))
}

func (srv *Server) setError(err error) {
	srv.mu.Lock()
	srv.error = err
	srv.mu.Unlock()
}

func (srv *Server) getError() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.error
}

func (srv *Server) watch(path string) error {
	return srv.watcher.Add(path, srv.watcherEvents)
}

func (srv *Server) unwatch(path string) error {
	return srv.watcher.Remove(path, srv.watcherEvents)
}

func (srv *Server) unwatchAll() error {
	srv.resources.Walk(srv.unwatch)
	srv.assets.Walk(srv.unwatch)
	for p := range srv.dep {
		srv.unwatch(p)
	}
	return nil
}

func (srv *Server) startWatcher() {
	var mu sync.Mutex
	var timer *time.Timer
	for {
		select {
		case event := <-srv.watcherEvents:
			mu.Lock()
			if timer != nil {
				timer.Stop()
				timer = nil
			}

			timer = time.AfterFunc(1*time.Second, func() {
				srv.sync(event.Name)
			})
			mu.Unlock()
		}
	}
}

func (srv *Server) runOnce() {
	srv.once.Do(func() {
		srv.busy <- true
		defer func() {
			srv.started <- true
			go srv.loop()
			<-srv.busy
		}()

		go srv.startWatcher()
		err := srv.build()
		srv.setError(err)

		if err == nil {
			err = srv.start()
			if err != nil {
				srv.setError(fmt.Errorf("%v\nError:%s\n", err, srv.stderr.ReadAll()))
			}
		}
		srv.resources.Walk(srv.watch)
		srv.assets.Walk(srv.watch)
	})
}

func newServer(context build.Context, conf serverConfig, proxyPort int, w *watcher.Watcher) (*Server, error) {
	var err error
	srv := new(Server)
	srv.conf = conf
	srv.proxyPort = proxyPort

	srv.resources, err = newResource(conf.Resources.Paths, conf.Resources.Ignore)

	if err != nil {
		return nil, err
	}

	srv.assets, err = newResource(conf.Assets.Paths, conf.Assets.Ignore)

	if err != nil {
		return nil, err
	}

	srv.startupTimeout = conf.StartupTimeout

	srv.target = strings.TrimSpace(conf.Target)
	srv.targetDir = filepath.Dir(conf.Target)
	srv.bin = strings.TrimSpace(conf.Bin)
	srv.builder = conf.Builder

	if !hasPrefix(srv.target, filepath.SplitList(context.GOPATH)) {
		// Target is not in the $GOPATH
		// Try to guess the import root(workspace) from the path
		roots := importRoots(srv.target)

		if len(roots) > 0 {
			context.GOPATH = strings.Join(append(roots, context.GOPATH), string(filepath.ListSeparator))
		}
	}

	srv.context = context

	if len(srv.builder) == 0 {
		gobin := "go"

		if len(context.GOROOT) > 0 && fileExists(context.GOROOT) {
			gobin = filepath.Join(context.GOROOT, "bin", gobin)
		}

		srv.builder = append(srv.builder, gobin, "build", "-o", srv.bin)
	}

	srv.watcher = w
	srv.watcherEvents = make(chan watcher.Event, 1)
	srv.ready = make(chan error, 1)
	srv.busy = make(chan bool, 1)
	srv.stopped = make(chan bool, 1)
	srv.started = make(chan bool, 1)
	srv.done = make(chan error, 1)
	srv.exit = make(chan bool, 1)
	srv.updateListeners = newUpdateListeners()
	srv.port = conf.Port
	srv.startup = conf.Startup
	srv.host = conf.Host
	srv.stdout = logger.NewLogWriter(os.Stdout, srv.host+"> ", log.LstdFlags)
	srv.stderr = new(logger.BufferedLogWriter)
	return srv, nil
}

func (srv *Server) testConnection(target *url.URL, timeout time.Duration) <-chan error {
	t := time.After(timeout)
	done := make(chan error, 1)
	client := &http.Client{}
	go func() {
		for {
			select {
			case err := <-srv.done:
				done <- err
				srv.done <- err
			case <-t:
				done <- errTimeout
				return
			default:
				response, err := client.Head(target.String())
				if err == nil {
					response.Body.Close()
					done <- nil
					return
				}

				if t, ok := err.(*url.Error); ok && t.Err == io.EOF {
					// The server started successfully but the handler paniced
					done <- nil
					return
				}

				if srv.getProcessState() != running {
					done <- nil
					return
				}

				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	return done
}

func (srv *Server) stopAndNotify() error {
	srv.busy <- true
	defer func() {
		srv.updateListeners.notify()
		srv.started <- <-srv.busy
	}()

	err := srv.stop()
	srv.setError(err)
	return err
}

func (srv *Server) stop() error {
	log.Printf("Stopping...%s:%v", srv.host, srv.port)
	select {
	case srv.stopped <- true:
		<-time.After(10 * time.Millisecond)
		srv.pending.Wait()
		return srv.stopProcess()
	default:
	}
	return nil
}

func (srv *Server) shutdown() error {
	srv.busy <- true
	log.Println("shuting down: ", srv.host)
	defer func() {
		<-srv.busy
	}()

	select {
	case srv.exit <- true:
		srv.unwatchAll()
		return srv.stop()
	default:
		return nil
	}
}

func (srv *Server) sync(filename string) error {
	srv.busy <- true
	notifyUpdate := true

	defer func() {
		if notifyUpdate {
			go srv.updateListeners.notify()
		}
		srv.started <- <-srv.busy
	}()

	_, rebuild := srv.dep[filename]

	restart := rebuild || srv.resources.MatchPath(filename)

	if !restart && !srv.assets.MatchPath(filename) {
		return nil
	}

	if restart {
		err := srv.stop()
		srv.setError(err)
		if err != nil {
			return err
		}
	}

	if rebuild {
		err := srv.build()
		srv.setError(err)

		if err != nil {
			return err
		}
	}

	if restart {
		// Let start handle the notification
		notifyUpdate = false
		err := srv.start()
		if err != nil {
			err = fmt.Errorf("%v\nError:%s\n", err, srv.stderr.ReadAll())
		}
		srv.setError(err)
	}

	return nil
}

func (srv *Server) start() error {
	log.Printf("Starting...%s", srv.host)
	defer srv.updateListeners.notify()

	if len(srv.addr) == 0 {
		srv.addr = net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
	}

	log.Println(srv.addr)

	if _, err := net.ResolveTCPAddr("tcp", srv.addr); err != nil {
		return err
	}

	srv.stderr.Reset()

	err := srv.startProcess()
	if err == nil {
		select {
		case err = <-srv.done:
		default:
		}
	}

	log.Println(srv.host, "...Startup completed")
	return err
}

func (srv *Server) loop() {
	<-srv.started
	err := srv.getError()
	for {
		select {
		case s := <-srv.stopped:
			srv.stopped <- s
			select {
			case <-srv.ready:
			default:
			}
			<-srv.started
			<-srv.stopped
			err = srv.getError()
		case <-srv.exit:
			select {
			case <-srv.ready:
			default:
			}
			return
		case srv.ready <- err:
		}
	}
}

func (srv *Server) stopProcess() (err error) {
	log.Println("Stopping process", srv.host)
	select {
	case err = <-srv.done:
		log.Println("Process already stopped")
	default:
		if srv.getProcessState() == running {
			srv.setProcessState(stopping)
			srv.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case err = <-srv.done:
			case <-time.After(srv.startupTimeout):
				srv.cmd.Process.Signal(syscall.SIGKILL)
				// TODO : We may need to set a timeout here
				err = <-srv.done
			}
		}
	}
	return err
}

func (srv *Server) restart() error {
	srv.busy <- true
	defer func() {
		srv.started <- <-srv.busy
	}()

	err := srv.stop()
	srv.setError(err)
	if err != nil {
		return err
	}

	err = srv.start()
	if err != nil {
		err = fmt.Errorf("%v\nError:%s\n", err, srv.stderr.ReadAll())
	}
	srv.setError(err)
	return err
}

func (srv *Server) startProcess() error {
	log.Println("Starting Process: ", srv.addr)
	srv.setProcessState(created)
	ev := env.New(os.Environ())
	for key, value := range srv.conf.Env {
		ev.Set(key, value)
	}

	cmd := exec.Command(srv.bin, srv.startup...)
	cmd.Env = ev.Data()
	cmd.Stderr = srv.stderr
	cmd.Stdout = srv.stdout

	err := cmd.Start()
	if err == nil {
		srv.setProcessState(running)
		srv.cmd = cmd

		go func() {
			status := cmd.Wait()

			log.Println(srv.host, "->", status)
			srv.done <- nil
			oldState := srv.getProcessState()
			srv.setProcessState(exited)

			select {
			case srv.busy <- true:
				if oldState == running {
					// The process crashed or was killed externally
					// Restart it
					// TODO: limit the number of consecutive restart
					if srv.stderr.Len() == 0 || strings.Contains(status.Error(), "terminated") {
						go srv.restart()
					} else {
						go srv.stopAndNotify()
					}
				}
				<-srv.busy
			default:
			}
		}()

		err = <-srv.testConnection(&url.URL{
			Host:   srv.addr,
			Scheme: "http",
			Path:   "/",
		}, srv.startupTimeout*time.Second)
	}
	return err

}

func (srv *Server) build() error {
	log.Printf("Building...%s", srv.host)

	// List of file to pass to "go build"
	var buildFiles []string

	dep, err := computeDep(&srv.context, srv.target)
	if err != nil {
		return err
	}

	for f := range srv.dep {
		srv.unwatch(f)
	}

	// Reset the dependency list.
	srv.dep = make(map[string]struct{})
	for _, f := range dep {
		srv.dep[f] = struct{}{}
		if err := srv.watch(f); err != nil {
			return err
		}

		if filepath.Dir(f) == srv.targetDir {
			buildFiles = append(buildFiles, filepath.Base(f))
		}
	}

	env := env.New(os.Environ())
	env.Set("GOPATH", srv.context.GOPATH)

	command, args := srv.builder[0], srv.builder[1:]

	args = append(args, buildFiles...)

	cmd := exec.Command(command, args...)
	cmd.Env = env.Data()
	cmd.Dir = srv.conf.WorkingDir

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

func (srv *Server) onUpdate() <-chan struct{} {
	return srv.updateListeners.register()
}

func (srv *Server) handleLivedevSocket(w http.ResponseWriter, r *http.Request) error {
	client, buf, err := w.(http.Hijacker).Hijack()

	if err == nil {
		// For now, no need for a full websocket protocol implementation
		// We need just enough to maintain the connection
		// Communicate changes to the caller by just closing the conntection
		go func() {
			code := http.StatusSwitchingProtocols
			fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
			buf.WriteString("Upgrade: websocket\r\n")
			buf.WriteString("Connection: Upgrade\r\n")
			fmt.Fprintf(buf, "Sec-WebSocket-Accept: %s\r\n", generateWebsocketAcceptKey(r.Header.Get("Sec-Websocket-Key")))
			fmt.Fprintf(buf, "Sec-WebSocket-Protocol: %s\r\n", liveReloadProtocol)
			buf.WriteString("\r\n")
			buf.Flush()
			done := make(chan bool, 1)
			update := srv.onUpdate()

			go func() {
				// We do not expect any message from the client
				// Any data indicates a connection closed or a misbehaving client
				client.Read(make([]byte, 8))
				done <- true
			}()

			select {
			case <-update:
			case <-done:
			}
			client.Close()
		}()
	}
	return err
}

func (srv *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) error {
	client, buf, err := w.(http.Hijacker).Hijack()

	if err != nil {
		return err
	}

	// We have taken over the connection from this point on. Do not return any error to the caller
	requestURL := *r.URL
	requestURL.Host = srv.addr

	conn, err := net.Dial(client.LocalAddr().Network(), requestURL.Host)

	if err != nil {
		writeWebSocketError(buf, err, http.StatusInternalServerError)
		client.Close()
		return nil
	}

	if err = r.Write(conn); err != nil {
		writeWebSocketError(buf, err, http.StatusInternalServerError)
		conn.Close()
		client.Close()
		return nil
	}

	go func() {
		done := make(chan bool, 1)
		copy := func(dst io.Writer, src io.Reader) {
			io.Copy(dst, src)
			done <- true
		}

		go copy(bufio.NewWriter(conn), buf)
		go copy(buf, bufio.NewReader(conn))

		<-done
		conn.Close()
		client.Close()
	}()

	return nil
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	srv.runOnce()
	isWS := r.Header.Get("Upgrade") == "websocket"
	isLiveReload := isWS && r.Header.Get("Sec-WebSocket-Protocol") == liveReloadProtocol

	err := <-srv.ready

	if isLiveReload {
		return srv.handleLivedevSocket(w, r)
	}

	if err != nil {
		return err
	}

	srv.pending.Add(1)
	defer srv.pending.Done()

	if isWS {
		return srv.serveWebSocket(w, r)
	}

	req := new(http.Request)
	*req = *r
	req.Host = srv.addr
	req.URL.Host = srv.addr
	req.Proto = "HTTP/1.1"
	req.Close = false
	req.ProtoMajor = 1
	req.ProtoMinor = 1

	if len(req.URL.Scheme) == 0 {
		req.URL.Scheme = "http"
	}

	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		req.Header.Set("X-Forwarded-For", ip)
	}

	transport := new(http.Transport)
	response, err := transport.RoundTrip(req)

	if err != nil {
		return fmt.Errorf("%s\n%s\n", err.Error(), srv.stderr.ReadAll())
	}

	defer response.Body.Close()

	wh := w.Header()

	for key, v := range response.Header {
		for _, value := range v {
			wh.Add(key, value)
		}
	}

	body := response.Body.(io.Reader)

	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		var contentLen int
		gzipped := "gzip" == response.Header.Get("Content-Encoding")
		if gzipped {
			body, err = gzip.NewReader(body)
			if err != nil {
				return err
			}
		}
		body, contentLen, err = appendLiveScript(body, srv.proxyPort)

		if err != nil {
			return err
		}
		if gzipped {
			gw := gzip.NewWriter(w)
			defer gw.Close()
			w = responseWriter{gw, w}
		}

		wh.Set("Content-Length", strconv.Itoa(contentLen))
	}

	w.WriteHeader(response.StatusCode)
	if _, err := io.Copy(w, body); err != nil {
		return fmt.Errorf("%s\n%s\n", err.Error(), srv.stderr.ReadAll())
	}

	return nil
}

func appendLiveScript(reader io.Reader, port int) (io.Reader, int, error) {

	data, err := ioutil.ReadAll(reader)

	if err == nil {
		data = appendHTML(data, []byte(fmt.Sprintf(liveReloadHTML, port)))
	}

	if err != nil {
		return nil, 0, err
	}

	return bytes.NewReader(data), len(data), nil

}

type responseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w responseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}
