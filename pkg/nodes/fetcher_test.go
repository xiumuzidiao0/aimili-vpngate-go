package nodes

import (
	"context"
	"errors"
	"testing"
)

func TestFetchNodesHonorsCanceledContext(t *testing.T) {
	fetcher := NewFetcher("https://example.invalid/api", "", NewSnapshotManager(t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetcher.FetchNodes(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
