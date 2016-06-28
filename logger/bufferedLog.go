package logger

import (
	"bytes"
	"sync"
)

// BufferedLogWriter represents a logging object that generates
// output to a buffer.
type BufferedLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Len returns the length of the buffer
func (l *BufferedLogWriter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Len()
}

// Reset resets the buffer to empty
func (l *BufferedLogWriter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Reset()
}

// WriteString writes a string to the buffer
func (l *BufferedLogWriter) WriteString(s string) (int, error) {
	return l.Write([]byte(s))
}

// Write writes the given data to the buffer
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

// ReadAll reads all the data from the buffer into a string
func (l *BufferedLogWriter) ReadAll() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.readAll()
}
