//go:build !linux

package proxy

import (
	"net"
	"time"
)

func dialUpstream(targetAddr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	return d.Dial("tcp", targetAddr)
}
