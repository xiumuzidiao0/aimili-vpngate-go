//go:build linux

package proxy

import (
	"net"
	"syscall"
	"time"
)

func dialUpstream(targetAddr string, devName string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var operr error
			fn := func(fd uintptr) {
				if devName != "" {
					// Dynamically bind to the specific tunnel device (tun0, tun1, tun2...)
					_ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, devName)
				}
			}
			if err := c.Control(fn); err != nil {
				return err
			}
			return operr
		},
	}
	return d.Dial("tcp", targetAddr)
}
