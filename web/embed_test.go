package web

import (
	"bytes"
	"testing"
)

func TestIndexUsesEscapedDynamicRendering(t *testing.T) {
	index, err := distFS.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}

	required := []byte("function escapeHtml(value)")
	if !bytes.Contains(index, required) {
		t.Fatal("embedded index is missing escapeHtml")
	}

	unsafePatterns := [][]byte{
		[]byte("${entry.message}"),
		[]byte(`toggleFavorite('${n.id}')`),
		[]byte(`stopTunnel('${t.id}')`),
		[]byte(`updateNodeOutbound('${n.name}'`),
		[]byte(`deleteSingBoxNode('${n.name}')`),
	}
	for _, pattern := range unsafePatterns {
		if bytes.Contains(index, pattern) {
			t.Fatalf("embedded index still contains unsafe HTML/JS interpolation: %q", pattern)
		}
	}
}
