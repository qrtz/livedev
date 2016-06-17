package logger

import (
	"io"
	"log"
)

// LogWriter is a wrapper for the standard logger to implement io.Writer
type LogWriter struct {
	*log.Logger
}

// NewLogWriter create a new LogWriter
func NewLogWriter(out io.Writer, prefix string, flag int) *LogWriter {
	return &LogWriter{log.New(out, prefix, flag)}
}

// Write writes the given data to the underlying logger
func (l *LogWriter) Write(b []byte) (int, error) {
	l.Logger.Printf("%s", b)
	return len(b), nil
}
