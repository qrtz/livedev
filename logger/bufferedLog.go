package logger

import (
	"bytes"
	"sync"
)

type BufferedLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *BufferedLogWriter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Reset()
}

func (l *BufferedLogWriter) WriteString(s string) (int, error) {
	return l.Write([]byte(s))
}

func (l *BufferedLogWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(b)
}

func (l *BufferedLogWriter) readAll() string {
	b := make([]byte, l.buf.Len())
	i, _ := l.buf.Read(b)
	return string(b[:i])
}

func (l *BufferedLogWriter) ReadAll() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.readAll()
}
