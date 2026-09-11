package tunnel

import (
	"net"
	"syscall"
	"time"
)

// CheckTunnelConnectivity tests whether a specific network interface has functional egress to the Internet.
func CheckTunnelConnectivity(devName string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	// Try port 80 and port 53 (TCP)
	targets := []string{"1.1.1.1:80", "1.0.0.1:80", "8.8.8.8:53"}
	for _, target := range targets {
		d := net.Dialer{
			Timeout: timeout,
			Control: func(network, address string, c syscall.RawConn) error {
				var operr error
				fn := func(fd uintptr) {
					if devName != "" {
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
		conn, err := d.Dial("tcp", target)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}
