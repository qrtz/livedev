package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func findAvailablePort() (*net.TCPAddr, error) {
	l, err := net.Listen("tcp", ":0")
	if err == nil {
		defer l.Close()
		addr := l.Addr()
		if a, ok := addr.(*net.TCPAddr); ok {
			return a, nil
		}
		return nil, fmt.Errorf("Unable to obtain a valid tcp port. %v", addr)
	}
	return nil, err
}

// HasPrefix is a convenient wrapper for strings.HasPrefix to test a list of prefixes
func HasPrefix(s string, prefixes []string) bool {
	if len(s) == 0 || len(prefixes) == 0 {
		return false
	}

	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}

	return false
}

// ImportRoots returns a list of directories that contain a sub-directory named "src"
func ImportRoots(path string) (roots []string) {
	dir, _ := filepath.Split(filepath.Clean(path))

	for i, p := len(dir)-1, len(dir); i > 0; i-- {
		if os.IsPathSeparator(dir[i]) {
			if dir[i+1:p] == "src" {
				roots = append(roots, dir[:i])
			}
			p = i
		}
	}

	return roots
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		return !os.IsNotExist(err)
	}

	return true
}

var tags = [][]byte{
	bytesReverse([]byte("</HTML>")),
	bytesReverse([]byte("</BODY>")),
}

func appendHTML(data, code []byte) ([]byte, error) {
	offset := len(data) - 1
DONE:
	for _, tag := range tags {
		for ; offset > len(tag) && isWhiteSpace(data[offset]); offset-- {
		}

		for i := 0; offset > 0 && i < len(tag); offset-- {
			b := data[offset]
			if !isWhiteSpace(b) {
				t := tag[i]
				if t >= 'A' && t <= 'Z' {
					b &= 0xDF
				}
				if b != t {
					// Not well-formed HTML
					offset = len(data) - 1
					break DONE
				}
				i += 1
			}
		}
	}

	if offset < len(data)-1 {
		offset += 1
	}
	data = append(data[:offset], append(code, data[offset:]...)...)
	return data, nil
}

func isWhiteSpace(b byte) bool {
	switch b {
	case ' ', '\n', '\t', '\r', '\x0c':
		return true
	}
	return false

}

func bytesReverse(b []byte) []byte {
	for i, c := range b[:len(b)/2] {
		b[i], b[len(b)-i-1] = b[len(b)-i-1], c
	}
	return b
}

func writeWebSocketError(w io.Writer, err error, code int) {
	b := bufio.NewWriter(w)
	fmt.Fprintf(b, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
	b.WriteString("\r\n")
	b.WriteString(err.Error())
	b.Flush()
}

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func generateWebsocketAcceptKey(key string) string {
	hash := sha1.New()
	hash.Write([]byte(key))
	hash.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}
