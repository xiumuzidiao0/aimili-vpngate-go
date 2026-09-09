//go:build linux

package proxy

import (
	"net"
	"syscall"
	"time"
)

func dialUpstream(targetAddr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var operr error
			fn := func(fd uintptr) {
				// Bind outgoing socket specifically to tun0 interface.
				// This ensures VPS default routing table (and SSH 22) is never modified,
				// while all proxy outbound traffic goes through the VPN tunnel.
				_ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, "tun0")
			}
			if err := c.Control(fn); err != nil {
				return err
			}
			return operr
		},
	}
	return d.Dial("tcp", targetAddr)
}
