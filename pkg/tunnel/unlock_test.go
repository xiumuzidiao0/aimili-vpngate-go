package tunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUnlockDetectorMock(t *testing.T) {
	tempDir := t.TempDir()
	detector := NewUnlockDetector(tempDir)

	// Verify empty cache
	if res := detector.GetUnlock("1.2.3.4"); res != nil {
		t.Fatalf("expected nil for uncached IP, got %v", res)
	}

	// Mock cached item
	cached := &UnlockResult{
		IP:        "1.2.3.4",
		OpenAI:    StatusUnlocked,
		Claude:    StatusUnlocked,
		Google:    StatusUnlocked,
		Netflix:   StatusUnlocked,
		CheckedAt: time.Now(),
	}
	detector.mu.Lock()
	detector.cache["1.2.3.4"] = cached
	detector.saveLocked()
	detector.mu.Unlock()

	// Re-load into fresh detector
	d2 := NewUnlockDetector(tempDir)
	res := d2.GetUnlock("1.2.3.4")
	if res == nil || res.OpenAI != StatusUnlocked {
		t.Fatalf("expected cached unlocked status, got %v", res)
	}
}

func TestMockProbing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := ts.Client()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("mock probing failed: %v", err)
	}
}
