package server

import (
	"net"
)

// GetServerAddress returns the full server address (host:port)
func GetServerAddress(host, port string) string {
	return net.JoinHostPort(host, port)
}
