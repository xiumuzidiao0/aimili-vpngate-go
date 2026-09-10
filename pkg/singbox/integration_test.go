package singbox

import (
	"context"
	"testing"
	"time"
)

func TestLiveClientExecution(t *testing.T) {
	client := NewClient()
	if !client.IsInstalled() {
		t.Skip("sing-box is not installed on this system, skipping live test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Test GetProtocols
	protos, err := client.GetProtocols(ctx)
	if err != nil {
		t.Fatalf("client.GetProtocols failed: %v", err)
	}
	if len(protos) == 0 {
		t.Fatalf("expected protocols > 0, got 0")
	}

	// 2. Test GetStatus
	status, err := client.GetStatus(ctx)
	if err != nil {
		t.Fatalf("client.GetStatus failed: %v", err)
	}
	if !status.Installed {
		t.Fatalf("expected installed true, got false")
	}

	// 3. Test ListNodes
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		t.Fatalf("client.ListNodes failed: %v", err)
	}
	t.Logf("Live test success: protocols=%d, core=%s, nodes=%d", len(protos), status.Core.Version, len(nodes))
}
