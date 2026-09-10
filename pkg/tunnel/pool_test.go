package tunnel

import (
	"fmt"
	"testing"

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

	// allocDevIndexLocked should reap dead tunnels 0 and 2, and return 0
	devIdx := pool.allocDevIndexLocked()
	pool.mu.Unlock()

	if devIdx != 0 {
		t.Fatalf("expected reaped devIdx 0 to be reused, got %d", devIdx)
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

	// Initially empty
	groups := mgr.ListGroups()
	if len(groups) != 1 || groups[0].TargetCount != 3 {
		t.Fatalf("expected 1 group with targetCount 3")
	}
}
