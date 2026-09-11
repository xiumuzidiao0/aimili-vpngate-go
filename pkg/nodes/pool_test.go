package nodes

import (
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
)

func TestIncrementalNodePoolAndEviction(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
	}

	np := NewNodePool(cfg)

	// 1. Initial State: empty store
	if len(np.GetCandidates()) != 0 {
		t.Fatalf("expected empty candidates initially")
	}

	// 2. Simulate First Pull: Node A and Node B
	now := time.Now()
	nodeA := &Node{
		ID:           "1.1.1.1:443",
		IP:           "1.1.1.1",
		Port:         443,
		CountryShort: "JP",
		Score:        1000,
		Speed:        50000000,
		FirstSeen:    now,
		LastSeen:     now,
	}
	nodeB := &Node{
		ID:           "2.2.2.2:443",
		IP:           "2.2.2.2",
		Port:         443,
		CountryShort: "US",
		Score:        2000,
		Speed:        60000000,
		FirstSeen:    now,
		LastSeen:     now,
	}

	np.MergeFreshNodesLocked([]*Node{nodeA, nodeB}, "source-1")
	if len(np.GetCandidates()) != 2 {
		t.Fatalf("expected 2 candidates after first merge, got %d", len(np.GetCandidates()))
	}

	// 3. Simulate Second Pull: Node B (updated score) and Node C (new node).
	// Node A is NOT in the second pull, but MUST NOT be overwritten or lost!
	nodeBUpdated := &Node{
		ID:           "2.2.2.2:443",
		IP:           "2.2.2.2",
		Port:         443,
		CountryShort: "US",
		Score:        3000, // Score updated from 2000 to 3000
		Speed:        70000000,
	}
	nodeC := &Node{
		ID:           "3.3.3.3:443",
		IP:           "3.3.3.3",
		Port:         443,
		CountryShort: "KR",
		Score:        1500,
		Speed:        40000000,
	}

	np.MergeFreshNodesLocked([]*Node{nodeBUpdated, nodeC}, "source-2")

	// Verify all 3 nodes exist in pool (Incremental preservation)
	candidates := np.GetCandidates()
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates (A preserved, B updated, C added), got %d", len(candidates))
	}

	nodeBStored := np.GetNodeByID("2.2.2.2:443")
	if nodeBStored == nil || nodeBStored.Score != 3000 {
		t.Fatalf("expected Node B to have updated score 3000, got %+v", nodeBStored)
	}

	nodeAStored := np.GetNodeByID("1.1.1.1:443")
	if nodeAStored == nil {
		t.Fatalf("expected Node A to still be preserved in pool even though missing from second pull")
	}

	// 4. Test Eviction of Stale / Dead Nodes
	// Mark Node A as dead and unseen for 48 hours
	np.mu.Lock()
	if n, ok := np.nodeStore["1.1.1.1:443"]; ok {
		n.LastSeen = now.Add(-48 * time.Hour)
		n.LatencyMs = -1
	}
	evicted := np.evictStaleNodesLocked(now)
	np.rebuildCandidatesLocked()
	np.mu.Unlock()

	if evicted != 1 {
		t.Fatalf("expected 1 dead stale node (Node A) to be evicted, got %d", evicted)
	}

	if np.GetNodeByID("1.1.1.1:443") != nil {
		t.Fatalf("expected Node A to be evicted from pool")
	}

	// 5. Test Favorite Protection from Eviction
	// Add dead Node D but mark as favorite
	nodeDFav := &Node{
		ID:           "4.4.4.4:443",
		IP:           "4.4.4.4",
		Port:         443,
		CountryShort: "JP",
		LatencyMs:    -1,
		LastSeen:     now.Add(-72 * time.Hour),
		FailCount:    10,
	}
	np.Favorites().Toggle("4.4.4.4:443") // Mark as favorite
	np.MergeFreshNodesLocked([]*Node{nodeDFav}, "test")

	np.mu.Lock()
	evictedFav := np.evictStaleNodesLocked(now)
	np.mu.Unlock()

	if evictedFav != 0 {
		t.Fatalf("expected favorite Node D to be protected from eviction, got %d evicted", evictedFav)
	}
	if np.GetNodeByID("4.4.4.4:443") == nil {
		t.Fatalf("expected favorite Node D to remain in pool")
	}

	// 6. Test Persistence: reload store from disk
	np2 := NewNodePool(cfg)
	if len(np2.GetCandidates()) == 0 {
		t.Fatalf("expected NodePool to reload saved nodes from disk")
	}
}
