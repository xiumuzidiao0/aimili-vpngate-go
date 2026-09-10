//go:build linux

package tunnel

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"time"
)

func newTunnelHTTPClient(devName string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: nil, // Do not use environment proxies
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{
				Timeout:   timeout,
				KeepAlive: 15 * time.Second,
				Control: func(netw, address string, c syscall.RawConn) error {
					var operr error
					fn := func(fd uintptr) {
						if devName != "" {
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
				d.Resolver = &net.Resolver{
					PreferGo: true,
					Dial: func(rctx context.Context, rnetw, raddr string) (net.Conn, error) {
						dnsDialer := net.Dialer{
							Timeout: 3 * time.Second,
							Control: func(n, a string, c syscall.RawConn) error {
								var err error
								fn := func(fd uintptr) {
									_ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, devName)
								}
								if err = c.Control(fn); err != nil {
									return err
								}
								return err
							},
						}
						return dnsDialer.DialContext(rctx, "udp", "8.8.8.8:53")
					},
				}
			}
			return d.DialContext(ctx, network, addr)
		},
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
