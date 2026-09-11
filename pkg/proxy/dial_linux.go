//go:build linux

package proxy

import (
	"context"
	"net"
	"syscall"
	"time"
)

func getInterfaceIPv4(devName string) net.IP {
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
		if ip4 := getInterfaceIPv4(devName); ip4 != nil {
			d.LocalAddr = &net.TCPAddr{IP: ip4}
		}

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
				if ip4 := getInterfaceIPv4(devName); ip4 != nil {
					dnsDialer.LocalAddr = &net.UDPAddr{IP: ip4}
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

	conn, err := d.Dial("tcp", targetAddr)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return conn, nil
}
