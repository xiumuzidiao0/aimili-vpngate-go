//go:build !linux

package tunnel

import (
	"net/http"
	"time"
)

func newTunnelHTTPClient(devName string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}
