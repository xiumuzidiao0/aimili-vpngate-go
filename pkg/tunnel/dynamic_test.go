package tunnel

import (
	"context"
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
)

type mockPrimaryConnector struct {
	connectedNode *nodes.Node
	status        string
}

func (m *mockPrimaryConnector) Connect(target *nodes.Node) error {
	m.connectedNode = target
	m.status = "connected"
	return nil
}

func (m *mockPrimaryConnector) GetActiveNode() *nodes.Node {
	return m.connectedNode
}

func (m *mockPrimaryConnector) GetStatus() string {
	return m.status
}

func TestUnlockFilterMatching(t *testing.T) {
	aiOnly := &nodes.UnlockResult{
		OpenAI:  nodes.StatusUnlocked,
		Claude:  nodes.StatusBlocked,
		Netflix: nodes.StatusBlocked,
		Google:  nodes.StatusBlocked,
	}

	streamOnly := &nodes.UnlockResult{
		OpenAI:  nodes.StatusBlocked,
		Claude:  nodes.StatusBlocked,
		Netflix: nodes.StatusUnlocked,
		Google:  nodes.StatusUnlocked,
	}

	fullUnlock := &nodes.UnlockResult{
		OpenAI:  nodes.StatusUnlocked,
		Claude:  nodes.StatusUnlocked,
		Netflix: nodes.StatusUnlocked,
		Google:  nodes.StatusUnlocked,
	}

	// 1. None filter matches everything
	if !aiOnly.MatchFilter("none") || !streamOnly.MatchFilter("none") {
		t.Fatalf("none filter should match all")
	}

	// 2. AI filter
	if !aiOnly.MatchFilter("ai") {
		t.Fatalf("aiOnly should match ai filter")
	}
	if streamOnly.MatchFilter("ai") {
		t.Fatalf("streamOnly should not match ai filter")
	}

	// 3. Streaming filter
	if !streamOnly.MatchFilter("streaming") {
		t.Fatalf("streamOnly should match streaming filter")
	}
	if aiOnly.MatchFilter("streaming") {
		t.Fatalf("aiOnly should not match streaming filter")
	}

	// 4. Full unlock filter
	if !fullUnlock.MatchFilter("full") {
		t.Fatalf("fullUnlock should match full filter")
	}
	if aiOnly.MatchFilter("full") || streamOnly.MatchFilter("full") {
		t.Fatalf("partial unlock should not match full filter")
	}
}

func TestSystemPrimaryGroupEvaluation(t *testing.T) {
	cfg := &config.Config{
		DataDir: t.TempDir(),
	}
	pool := NewPool(cfg, nil)
	np := nodes.NewNodePool(cfg)
	mgr := NewDynamicGroupManager(cfg, pool, np)

	mockPC := &mockPrimaryConnector{}
	mgr.SetPrimaryConnector(mockPC)

	// Inject candidate nodes
	np.SetCandidatesForTest([]*nodes.Node{
		{
			ID:           "node-us-ai",
			IP:           "1.1.1.1",
			CountryShort: "US",
			Score:        1000,
			Unlock: &nodes.UnlockResult{
				OpenAI: nodes.StatusUnlocked,
			},
		},
		{
			ID:           "node-jp-stream",
			IP:           "2.2.2.2",
			CountryShort: "JP",
			Score:        2000,
			Unlock: &nodes.UnlockResult{
				Netflix: nodes.StatusUnlocked,
			},
		},
	})

	sysGroup := mgr.GetGroup(SystemPrimaryGroupID)
	if sysGroup == nil || !sysGroup.IsSystem {
		t.Fatalf("expected system-primary group to exist and be system")
	}

	// Set criteria: Japan only
	sysGroup.Country = "JP"
	sysGroup.UnlockFilter = "streaming"
	_ = mgr.SaveGroup(sysGroup)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.EvaluateGroup(ctx, sysGroup)

	if mockPC.connectedNode == nil {
		t.Fatalf("expected primary connector to be called with JP streaming node")
	}
	if mockPC.connectedNode.ID != "node-jp-stream" {
		t.Fatalf("expected node-jp-stream, got %s", mockPC.connectedNode.ID)
	}
}
