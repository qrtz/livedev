package main

import (
	"sync"

	"github.com/fsnotify/fsnotify"
)

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

func newFileWatcher() (*fileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return nil, err
	}

	return &fileWatcher{Watcher: watcher}, nil

}
