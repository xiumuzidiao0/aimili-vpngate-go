package web

import (
	"bytes"
	"testing"
)

func TestIndexUsesEscapedDynamicRendering(t *testing.T) {
	script, err := distFS.ReadFile("dist/app.js")
	if err != nil {
		t.Fatalf("read embedded script: %v", err)
	}

	required := []byte("function escapeHtml(value)")
	if !bytes.Contains(script, required) {
		t.Fatal("embedded script is missing escapeHtml")
	}

	unsafePatterns := [][]byte{
		[]byte("${entry.message}"),
		[]byte(`toggleFavorite('${n.id}')`),
		[]byte(`stopTunnel('${t.id}')`),
		[]byte(`updateNodeOutbound('${n.name}'`),
		[]byte(`deleteSingBoxNode('${n.name}')`),
	}
	for _, pattern := range unsafePatterns {
		if bytes.Contains(script, pattern) {
			t.Fatalf("embedded script still contains unsafe HTML/JS interpolation: %q", pattern)
		}
	}
}

func TestEmbeddedUIAssets(t *testing.T) {
	for _, path := range []string{
		"dist/index.html",
		"dist/styles.css",
		"dist/app.js",
		"dist/favicon.svg",
		"dist/fonts/geist-400.ttf",
	} {
		if _, err := distFS.ReadFile(path); err != nil {
			t.Fatalf("embedded asset %s is unavailable: %v", path, err)
		}
	}
}
