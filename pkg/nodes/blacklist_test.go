package nodes

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestBlacklistRevival(t *testing.T) {
	dataDir := t.TempDir()
	bm := NewBlacklistManager(dataDir)

	// 1. Start a local dummy TCP listener to simulate a revived node
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	_, pStr, _ := net.SplitHostPort(ln.Addr().String())
	livePort, _ := strconv.Atoi(pStr)

	// 2. Mark the live node as blacklisted
	liveNode := &Node{
		ID:           "live-node-1",
		IP:           "127.0.0.1",
		Port:         livePort,
		Proto:        "tcp",
		CountryShort: "JP",
	}
	bm.Mark(liveNode, "测试暂时故障", 15*time.Minute)

	// 3. Mark an unreachable dead node
	deadNode := &Node{
		ID:           "dead-node-2",
		IP:           "127.0.0.1",
		Port:         59998, // assuming closed
		Proto:        "tcp",
		CountryShort: "US",
	}
	bm.Mark(deadNode, "彻底断开", 15*time.Minute)

	if bm.Count() != 2 {
		t.Fatalf("expected 2 blacklisted nodes initially, got %d", bm.Count())
	}

	// 4. Run probe and revive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	revived, err := bm.ProbeAndRevive(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}

	if len(revived) != 1 || revived[0].ID != "live-node-1" {
		t.Fatalf("expected 1 revived node ('live-node-1'), got %d", len(revived))
	}

	// Verify live node is no longer blacklisted
	if bm.IsBlacklisted("live-node-1") {
		t.Fatalf("expected live-node-1 to be un-blacklisted")
	}

	// Verify dead node remains blacklisted
	if !bm.IsBlacklisted("dead-node-2") {
		t.Fatalf("expected dead-node-2 to remain blacklisted")
	}
}
