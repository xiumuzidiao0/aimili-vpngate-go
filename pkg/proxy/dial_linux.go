//go:build linux

package proxy

import (
	"context"
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

	if devName != "" {
		// Resolve DNS specifically over the target VPN tunnel device to prevent DNS leaks and host DNS pollution
		d.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				dnsDialer := net.Dialer{
					Timeout: 4 * time.Second,
					Control: func(netw, addr string, c syscall.RawConn) error {
						var operr error
						fn := func(fd uintptr) {
							_ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, devName)
						}
						if err := c.Control(fn); err != nil {
							return err
						}
						return operr
					},
				}
				return dnsDialer.DialContext(ctx, "udp", "8.8.8.8:53")
			},
		}
	}

	return d.Dial("tcp", targetAddr)
}
