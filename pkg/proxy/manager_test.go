package proxy

import (
	"os"
	"testing"

	"aimili-vpngate-go/pkg/config"
)

func TestPortRulesFilePermissions(t *testing.T) {
	cfg := &config.Config{
		DataDir:   t.TempDir(),
		ProxyPort: 7928,
	}
	manager := NewMultiPortManager(cfg, nil, nil)

	info, err := os.Stat(manager.rulesPath)
	if err != nil {
		t.Fatalf("stat port rules: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected port rules mode 0600, got %o", perm)
	}
}
