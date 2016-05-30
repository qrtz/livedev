package logger

import (
	"io"
	"log"
)

type LogWriter struct {
	log *log.Logger
}

func NewLogWriter(out io.Writer, prefix string, flag int) *LogWriter {
	return &LogWriter{log: log.New(out, prefix, flag)}
}

func (l *LogWriter) Write(b []byte) (int, error) {
	l.log.Printf("%s", b)
	return len(b), nil
}
