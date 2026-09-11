package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
)

func TestTunnelListStableSorting(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	pool := NewPool(cfg, nil)

	// Inject mock tunnels with out-of-order dev indices
	pool.mu.Lock()
	pool.tunnels["tun-3"] = &Tunnel{ID: "tun-3", DevName: "tun3", DevIndex: 3, Status: StatusConnected}
	pool.tunnels["tun-0"] = &Tunnel{ID: "tun-0", DevName: "tun0", DevIndex: 0, Status: StatusConnected}
	pool.tunnels["tun-2"] = &Tunnel{ID: "tun-2", DevName: "tun2", DevIndex: 2, Status: StatusConnected}
	pool.tunnels["tun-1"] = &Tunnel{ID: "tun-1", DevName: "tun1", DevIndex: 1, Status: StatusConnected}
	pool.mu.Unlock()

	tunnels := pool.ListTunnels()
	if len(tunnels) != 4 {
		t.Fatalf("expected 4 tunnels, got %d", len(tunnels))
	}

	for i, tun := range tunnels {
		expectedDev := fmt.Sprintf("tun%d", i)
		if tun.DevName != expectedDev || tun.DevIndex != i {
			t.Errorf("at index %d expected %s, got %s (devIdx: %d)", i, expectedDev, tun.DevName, tun.DevIndex)
		}
	}
}

func TestDevIndexRecyclingOnDeadTunnels(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	pool := NewPool(cfg, nil)

	pool.mu.Lock()
	// Mark indices 0, 1, 2 as used by tunnels
	pool.usedDevs[0] = true
	pool.tunnels["tun-0"] = &Tunnel{ID: "tun-0", DevName: "tun0", DevIndex: 0, Status: StatusFailed}
	pool.usedDevs[1] = true
	pool.tunnels["tun-1"] = &Tunnel{ID: "tun-1", DevName: "tun1", DevIndex: 1, Status: StatusConnected}
	pool.usedDevs[2] = true
	pool.tunnels["tun-2"] = &Tunnel{ID: "tun-2", DevName: "tun2", DevIndex: 2, Status: StatusStopped}
	pool.mu.Unlock()

	// Cleanup dead tunnels 0 and 2, then alloc should reuse 2 (0 is reserved for primary).
	pool.ReapStaleTunnels()
	pool.mu.Lock()
	devIdx := pool.allocConcurrentDevIndexLocked()
	pool.mu.Unlock()

	if devIdx != 2 {
		t.Fatalf("expected reaped concurrent devIdx 2 to be reused, got %d", devIdx)
	}

	// Only active tunnel 1 should remain in pool
	pool.mu.RLock()
	if len(pool.tunnels) != 1 || pool.tunnels["tun-1"] == nil {
		t.Errorf("expected dead tunnels to be removed from pool map, got %d tunnels", len(pool.tunnels))
	}
	pool.mu.RUnlock()
}

func TestDynamicGroupFilteringDeadTunnels(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	pool := NewPool(cfg, nil)
	np := nodes.NewNodePool(cfg)
	mgr := NewDynamicGroupManager(cfg, pool, np)

	// Save group with target count 3
	g := &DynamicGroup{
		ID:          "dg-test",
		Name:        "测试组",
		Enabled:     true,
		TargetCount: 3,
		Country:     "JP",
	}
	_ = mgr.SaveGroup(g)

	// Should have 2 groups: system-primary (TargetCount 1) and dg-test (TargetCount 3)
	groups := mgr.ListGroups()
	if len(groups) != 2 || groups[0].ID != SystemPrimaryGroupID || groups[1].TargetCount != 3 {
		t.Fatalf("expected 2 groups with system-primary first and dg-test targetCount 3, got %+v", groups)
	}
}

func TestPrimaryTunnelReservation(t *testing.T) {
	cfg := &config.Config{
		DataDir:        t.TempDir(),
		OpenVPNCommand: "true",
	}
	pool := NewPool(cfg, nil)

	// 1. Concurrent tunnel starts first
	node1 := &nodes.Node{ID: "node-1", IP: "1.1.1.1", CountryShort: "JP", ConfigData: "client\n"}
	tun1, err := pool.StartTunnel(node1)
	if err != nil {
		t.Fatalf("failed to start concurrent tunnel: %v", err)
	}

	// Concurrent tunnel MUST allocate tun1, NOT tun0!
	if tun1.DevIndex != 1 || tun1.DevName != "tun1" {
		t.Fatalf("expected concurrent tunnel to be tun1 (devIndex 1), got %s (devIndex %d)", tun1.DevName, tun1.DevIndex)
	}

	// 2. Primary tunnel starts second
	nodePrimary := &nodes.Node{ID: "node-primary", IP: "2.2.2.2", CountryShort: "US", ConfigData: "client\n"}
	tunPrimary, err := pool.StartPrimaryTunnel(nodePrimary)
	if err != nil {
		t.Fatalf("failed to start primary tunnel: %v", err)
	}

	// Primary tunnel MUST allocate tun0 (devIndex 0)!
	if tunPrimary.DevIndex != 0 || tunPrimary.DevName != "tun0" {
		t.Fatalf("expected primary tunnel to be tun0 (devIndex 0), got %s (devIndex %d)", tunPrimary.DevName, tunPrimary.DevIndex)
	}

	// Clean up
	_ = pool.StopTunnel(tun1.ID)
	_ = pool.StopTunnel(tunPrimary.ID)
}

func TestCleanupTunnelIsIdempotentAndReleasesDevice(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	pool := NewPool(cfg, nil)
	tun := &Tunnel{
		ID:       "tun-cleanup",
		DevName:  "tun2",
		DevIndex: 2,
		Status:   StatusFailed,
		done:     make(chan struct{}),
	}

	pool.mu.Lock()
	pool.tunnels[tun.ID] = tun
	pool.usedDevs[tun.DevIndex] = true
	pool.mu.Unlock()

	pool.cleanupTunnel(tun, tun.ID)
	pool.cleanupTunnel(tun, tun.ID)

	select {
	case <-tun.done:
	default:
		t.Fatal("cleanup did not close the completion channel")
	}

	pool.mu.RLock()
	_, exists := pool.tunnels[tun.ID]
	used := pool.usedDevs[tun.DevIndex]
	pool.mu.RUnlock()
	if exists || used {
		t.Fatalf("cleanup did not release tunnel state: exists=%v used=%v", exists, used)
	}
}

func TestTunnelSnapshotCopiesUnlock(t *testing.T) {
	tun := &Tunnel{ID: "tun-unlock", Status: StatusConnected}
	result := &UnlockResult{OpenAI: StatusUnlocked, CheckedAt: time.Now()}
	tun.SetUnlock(result)

	snapshot := tun.Snapshot()
	result.OpenAI = StatusBlocked
	if snapshot.Unlock == nil || snapshot.Unlock.OpenAI != StatusUnlocked {
		t.Fatal("snapshot retained a mutable unlock pointer")
	}
}

func TestStopTunnelWaitsForCleanup(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	scriptPath := filepath.Join(t.TempDir(), "fake-openvpn.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 30\n"), 0700); err != nil {
		t.Fatalf("write fake openvpn: %v", err)
	}
	cfg.OpenVPNCommand = scriptPath

	pool := NewPool(cfg, nil)
	tun, err := pool.StartTunnel(&nodes.Node{
		ID:           "node-stop",
		IP:           "192.0.2.1",
		CountryShort: "JP",
		ConfigData:   "client\nremote 192.0.2.1 1194 udp\n",
	})
	if err != nil {
		t.Fatalf("StartTunnel failed: %v", err)
	}
	if err := pool.StopTunnel(tun.ID); err != nil {
		t.Fatalf("StopTunnel failed: %v", err)
	}

	pool.mu.RLock()
	_, exists := pool.tunnels[tun.ID]
	used := pool.usedDevs[tun.DevIndex]
	pool.mu.RUnlock()
	if exists || used {
		t.Fatalf("tunnel resources not released: exists=%v used=%v", exists, used)
	}

	select {
	case <-tun.done:
	default:
		t.Fatal("cleanup completion channel was not closed")
	}
}
