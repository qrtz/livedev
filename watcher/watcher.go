package watcher

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Event wraps fsnotify.Event
type Event struct {
	fsnotify.Event
}

// Watcher wraps fsnotify.Watcher.
type Watcher struct {
	watcher *fsnotify.Watcher
	mu      sync.RWMutex
	watches map[string][]chan<- Event
	stop    chan struct{}
}

// New creates a new Watcher and begins watching events
func New() (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher: watcher,
		watches: make(map[string][]chan<- Event),
		stop:    make(chan struct{}, 1),
	}
	go w.run()
	return w, nil
}

func (w *Watcher) run() {
	for {
		select {
		case <-w.stop:
			return
		case event := <-w.watcher.Events:
			if event.Op&fsnotify.Chmod != fsnotify.Chmod {
				w.notify(Event{event})
			}
		case err := <-w.watcher.Errors:
			log.Println("Watcher error:", err)
		}
	}
}

func (w *Watcher) notify(e Event) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := filepath.Clean(e.Name)

	for key, channels := range w.watches {
		if strings.HasPrefix(p, key) {
			for _, ch := range channels {
				go func(c chan<- Event) {
					defer func() {
						// Catch potential send on closed channel
						recover()
					}()
					select {
					case c <- e:
					default:
						log.Println("Unable to Notify: ", e.Name)
					}
				}(ch)
			}
		}
	}
}

// Close stops all watches and closes the underlying watcher.
func (w *Watcher) Close() error {
	select {
	case w.stop <- struct{}{}:
		return w.watcher.Close()
	default:
		return errors.New("Already stopped")
	}
}

// Add registers a channel for events on the given path
func (w *Watcher) Add(path string, ch chan<- Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path = filepath.Clean(path)
	chs, exists := w.watches[path]

	if exists {
		for _, c := range chs {
			if c == ch {
				return nil
			}
		}
	}

	err := w.watcher.Add(path)

	if err == nil {
		w.watches[path] = append(chs, ch)
	}

	return err
}

// Remove unregisters a channel for events on the given path
func (w *Watcher) Remove(path string, ch chan<- Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path = filepath.Clean(path)
	chs, exists := w.watches[path]

	if exists {
		for i, c := range chs {
			if c == ch {
				err := w.watcher.Remove(path)
				if err == nil {
					w.watches[path] = append(chs[0:i], chs[i+1:]...)
				}
				return err
			}
		}

		if len(chs) == 0 {
			delete(w.watches, path)
		}
	}
	return nil
}
