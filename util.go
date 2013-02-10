package main

import (
	"fmt"
	"net"
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
