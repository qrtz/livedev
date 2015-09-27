package main

import (
	"fmt"
	"net"
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

// Test whether any element in prefix begins with path
func hasPrefix(path string, prefix []string) bool {
	if len(path) == 0 || len(prefix) == 0 {
		return false
	}

	for _, p := range prefix {
		p = strings.TrimSpace(p)

		if len(p) > 0 && strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}

// Returns a list of directory names with a sub-directory named "src"
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
