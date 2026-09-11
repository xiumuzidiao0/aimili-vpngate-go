package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
)

func TestPortListenerHonorsConfiguredProxyHost(t *testing.T) {
	cfg := &config.Config{
		ProxyHost:           "127.0.0.1",
		ProxyMaxConnections: 16,
	}
	listener := NewPortListener(PortRule{Port: 0, Enabled: true}, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = listener.Start(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listener.mu.Lock()
		bound := listener.listener
		listener.mu.Unlock()
		if bound != nil {
			addr, ok := bound.Addr().(*net.TCPAddr)
			if !ok {
				t.Fatalf("unexpected listener address type %T", bound.Addr())
			}
			if !addr.IP.Equal(net.ParseIP("127.0.0.1")) {
				t.Fatalf("listener ignored ProxyHost and bound to %s", bound.Addr())
			}
			listener.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("listener did not start")
}
