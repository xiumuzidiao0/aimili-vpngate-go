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
					if err := syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, devName); err != nil {
						operr = err
					}
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
							if err := syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, devName); err != nil {
								operr = err
							}
						}
						if err := c.Control(fn); err != nil {
							return err
						}
						return operr
					},
				}
				conn, err := dnsDialer.DialContext(ctx, "udp", "8.8.8.8:53")
				if err != nil {
					// Fallback to Cloudflare DNS
					return dnsDialer.DialContext(ctx, "udp", "1.1.1.1:53")
				}
				return conn, nil
			},
		}
	}

	return d.Dial("tcp", targetAddr)
}
