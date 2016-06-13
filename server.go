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

	"github.com/fsnotify/fsnotify"
	"github.com/qrtz/livedev/env"
	"github.com/qrtz/livedev/logger"
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

type Resource struct {
	Ignore *regexp.Regexp
	Paths  map[string]struct{}
}

type fileWatcher struct {
	*fsnotify.Watcher

	mu       sync.RWMutex
	isClosed bool
}

func (w *fileWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.isClosed {
		return nil
	}
	w.isClosed = true
	return w.Watcher.Close()
}

func (w *fileWatcher) IsClosed() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isClosed

}

func (r Resource) Watch(watcher *fileWatcher) {
	for f := range r.Paths {
		if info, err := os.Lstat(f); err == nil {
			if info.IsDir() {
				filepath.Walk(f, func(path string, info os.FileInfo, err error) error {
					if err == nil && info.IsDir() {
						if r.Ignore != nil && r.Ignore.MatchString(path) {
							return filepath.SkipDir
						}
						watcher.Add(path)
					}
					return nil
				})
			}
			watcher.Add(f)
		}
	}
}

func (r Resource) MatchPath(p string) bool {
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

type Server struct {
	addr           string
	bin            string
	builder        []string
	closed         chan struct{}
	context        build.Context
	dep            map[string]struct{}
	host           string
	port           int
	resources      Resource
	assets         Resource
	startup        []string
	target         string
	targetDir      string
	stdout         *logger.LogWriter
	stderr         *logger.BufferedLogWriter
	startupTimeout time.Duration
	watcher        *fileWatcher
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

func (srv *Server) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return err
	}

	srv.watcher = &fileWatcher{Watcher: watcher}

	go func() {
		var mu sync.Mutex
		var timer *time.Timer
		for {
			select {
			case event := <-srv.watcher.Events:
				mu.Lock()

				if event.Op&fsnotify.Chmod != fsnotify.Chmod {
					if timer != nil {
						timer.Stop()
						timer = nil
					}

					timer = time.AfterFunc(1*time.Second, func() {
						srv.Sync(event.Name)
					})
				}
				mu.Unlock()
			case err := <-srv.watcher.Errors:
				log.Println("Watcher error:", err, srv.watcher.IsClosed())
				if srv.watcher.IsClosed() {
					return
				}
			}
		}
	}()

	return nil
}

func (srv *Server) runOnce() {
	srv.once.Do(func() {
		srv.busy <- true
		defer func() {
			srv.started <- true
			go srv.loop()
			<-srv.busy
		}()

		// TODO: Use only 1 watcher for all servers
		if err := srv.startWatcher(); err != nil {
			srv.setError(err)
			return
		}

		err := srv.build()
		srv.setError(err)

		if err == nil {
			err = srv.start()
			if err != nil {
				srv.setError(fmt.Errorf("%v\nError:%s\n", err, srv.stderr.ReadAll()))
			}
		}
		srv.resources.Watch(srv.watcher)
		srv.assets.Watch(srv.watcher)
	})
}

func NewServer(context build.Context, s serverConfig) (*Server, error) {
	srv := new(Server)

	if len(s.Resources.Paths) > 0 {
		var ignore *regexp.Regexp
		paths := make(map[string]struct{})
		var v struct{}
		for _, s := range s.Resources.Paths {
			if p := strings.TrimSpace(s); len(p) > 0 {
				paths[filepath.Clean(p)] = v
			}
		}

		if len(paths) > 0 {
			if s := strings.TrimSpace(s.Resources.Ignore); len(s) > 0 {
				if pattern, err := regexp.Compile(s); err == nil {
					ignore = pattern
				} else {
					return nil, errInvalidFilePattern
				}
			}

			srv.resources = Resource{ignore, paths}
		}
	}

	if len(s.Assets.Paths) > 0 {
		var ignore *regexp.Regexp
		paths := make(map[string]struct{})
		var v struct{}
		for _, s := range s.Assets.Paths {
			if p := strings.TrimSpace(s); len(p) > 0 {
				paths[filepath.Clean(p)] = v
			}
		}

		if len(paths) > 0 {
			if s := strings.TrimSpace(s.Assets.Ignore); len(s) > 0 {
				if pattern, err := regexp.Compile(s); err == nil {
					ignore = pattern
				} else {
					return nil, errInvalidFilePattern
				}
			}

			srv.assets = Resource{ignore, paths}
		}
	}

	srv.startupTimeout = s.StartupTimeout

	if srv.startupTimeout == 0 {
		srv.startupTimeout = 10
	}

	srv.target = strings.TrimSpace(s.Target)
	srv.targetDir = filepath.Dir(s.Target)
	srv.bin = strings.TrimSpace(s.Bin)
	srv.builder = s.Builder

	if len(srv.bin) == 0 {
		srv.bin = filepath.Join(os.TempDir(), "livedev-"+s.Host)
	}

	if !HasPrefix(srv.target, filepath.SplitList(context.GOPATH)) {
		// Target is not in the $GOPATH
		// Try to guess the import root(workspace) from the path
		roots := ImportRoots(srv.target)

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

	srv.ready = make(chan error, 1)
	srv.busy = make(chan bool, 1)
	srv.stopped = make(chan bool, 1)
	srv.started = make(chan bool, 1)
	srv.done = make(chan error, 1)
	srv.exit = make(chan bool, 1)
	srv.updateListeners = newUpdateListeners()
	srv.port = s.Port
	srv.startup = s.Startup
	srv.host = s.Host
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
					// The server started successfully but
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
		srv.started <- <-srv.busy
	}()

	select {
	case srv.exit <- true:
		if srv.watcher != nil {
			srv.watcher.Close()
		}
		return srv.stop()
	default:
		return nil
	}
}

func (srv *Server) Sync(filename string) error {
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

	generatePort := srv.port == 0

	if generatePort {
		addr, err := findAvailablePort()

		if err != nil {
			return err
		}
		srv.port = addr.Port
	}

	if len(srv.addr) == 0 {
		srv.addr = net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
	}
	log.Println(srv.addr)

	if _, err := net.ResolveTCPAddr("tcp", srv.addr); err != nil {
		return err
	}

	srv.stderr.Reset()

	// The server must accept "--addr" argument if no port is specified in the configuration
	if generatePort {
		srv.startup = append(srv.startup, "--addr", srv.addr)
	}

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
	log.Println("Stopping process")
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
					// The process crashed or was kill externally
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
		srv.watcher.Remove(f)
	}

	// Reset the dependency list.
	srv.dep = make(map[string]struct{})
	for _, f := range dep {
		srv.dep[f] = struct{}{}
		if err := srv.watcher.Add(f); err != nil {
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
	cmd.Dir = srv.targetDir

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

	if err := <-srv.ready; err != nil {
		if isLiveReload {
			return srv.handleLivedevSocket(w, r)
		}
		return err
	}

	if isLiveReload {
		return srv.handleLivedevSocket(w, r)
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
		if response.Header.Get("Content-Encoding") == "gzip" {
			body, err = gzip.NewReader(body)
			if err != nil {
				return err
			}
		}

		data, err := ioutil.ReadAll(body)
		if err == nil {
			data, err = appendHTML(data, []byte(liveReloadHTML))
		}

		if err != nil {
			return err
		}

		wh.Set("Content-Length", strconv.Itoa(len(data)))
		body = bytes.NewReader(data)
	}

	w.WriteHeader(response.StatusCode)
	if _, err := io.Copy(w, body); err != nil {
		return fmt.Errorf("%s\n%s\n", err.Error(), srv.stderr.ReadAll())
	}

	return nil
}
