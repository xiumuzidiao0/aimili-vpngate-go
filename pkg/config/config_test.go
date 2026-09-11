package config

import (
	"os"
	"testing"
)

func TestLoadConfigUsesLoopbackAndSecureDataDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("UI_HOST", "")

	cfg := LoadConfig()
	if cfg.UIHost != "127.0.0.1" {
		t.Fatalf("expected loopback UI host, got %q", cfg.UIHost)
	}

	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Fatalf("expected data dir mode 0700, got %o", perm)
	}
}

func TestUpdateSettingsRejectsEnvInjection(t *testing.T) {
	cfg := &Config{DataDir: t.TempDir()}
	err := cfg.UpdateSettings(SettingsDTO{
		UIUsername: "admin\nEVIL=1",
	})
	if err == nil {
		t.Fatal("expected newline in setting to be rejected")
	}
}
