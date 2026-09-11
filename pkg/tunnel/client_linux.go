//go:build linux

package tunnel

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"time"
)

func getTunnelInterfaceIPv4(devName string) net.IP {
	if devName == "" {
		return nil
	}
	iface, err := net.InterfaceByName(devName)
	if err != nil {
		return nil
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4
			}
		}
	}
	return nil
}

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
				if ip4 := getTunnelInterfaceIPv4(devName); ip4 != nil {
					d.LocalAddr = &net.TCPAddr{IP: ip4}
				}

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
						if ip4 := getTunnelInterfaceIPv4(devName); ip4 != nil {
							dnsDialer.LocalAddr = &net.UDPAddr{IP: ip4}
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
