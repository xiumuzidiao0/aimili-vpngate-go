package nodes

import (
	"testing"
)

func TestReputationManager(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewReputationManager(tempDir)

	ip := "198.51.100.1"
	nodeID := "198.51.100.1:1194"

	// Initial score should be 60
	if s := rm.GetScore(ip); s != 60 {
		t.Fatalf("expected initial score 60, got %d", s)
	}

	// Record a success
	rm.RecordSuccess(ip, nodeID)
	if s := rm.GetScore(ip); s != 65 {
		t.Fatalf("expected score 65 after success, got %d", s)
	}

	// Record uptime 3600 seconds (2 x 1800s -> +10)
	rm.RecordUptime(ip, nodeID, 3600)
	if s := rm.GetScore(ip); s != 67 { // 65 + 2 = 67 (3600/1800 = 2)
		t.Fatalf("expected score 67 after uptime, got %d", s)
	}

	// Record a quick drop failure (-25)
	rm.RecordFail(ip, nodeID, true)
	if s := rm.GetScore(ip); s != 42 {
		t.Fatalf("expected score 42 after quick drop fail, got %d", s)
	}

	// Re-load from disk
	rm2 := NewReputationManager(tempDir)
	if s := rm2.GetScore(ip); s != 42 {
		t.Fatalf("expected persisted score 42, got %d", s)
	}
}
