package vpn

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func CheckTUNDevice() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	info, err := os.Stat("/dev/net/tun")
	if err != nil {
		return fmt.Errorf("tun device /dev/net/tun not found, please ensure TUN/TAP kernel module is enabled: %w", err)
	}

	if info.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("/dev/net/tun is not a valid device")
	}

	return nil
}

func KillStrayOpenVPN() {
	if runtime.GOOS != "linux" {
		return
	}
	// Try killall openvpn safely
	_ = exec.Command("killall", "-9", "openvpn").Run()
}

func CheckExternalConnectivity(timeout time.Duration) bool {
	return CheckTunnelConnectivity("tun0", timeout)
}

func CheckTunnelConnectivity(devName string, timeout time.Duration) bool {
	targets := []string{"1.1.1.1:53", "8.8.8.8:53", "api.ipify.org:80"}
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
