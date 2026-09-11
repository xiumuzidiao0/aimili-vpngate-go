package server

import (
	"context"
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/singbox"
)

func TestWatchdogCooldownAndLifecycle(t *testing.T) {
	cfg := &config.Config{
		DataDir: t.TempDir(),
	}
	s := &Server{
		cfg:           cfg,
		singboxClient: singbox.NewClient(),
	}

	wd := NewSingBoxWatchdog(s, 100*time.Millisecond)

	// Verify cooldown logic
	wd.mu.Lock()
	wd.cooldownUntil = time.Now().Add(1 * time.Hour)
	wd.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Should safely return without error because of cooldown
	wd.check(ctx)

	wd.mu.Lock()
	if wd.consecutiveFails != 0 {
		t.Fatalf("expected 0 fails during cooldown")
	}
	wd.mu.Unlock()
}
