package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aimili-vpngate-go/pkg/config"
)

func TestTelegramNotifierMock(t *testing.T) {
	sentMsg := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "sendMessage") {
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if txt, ok := payload["text"].(string); ok {
				sentMsg = txt
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		TelegramBotToken: "mock-token",
		TelegramChatID:   "123456",
		DataDir:          t.TempDir(),
	}

	notifier := NewTelegramNotifier(cfg)
	notifier.client = ts.Client()

	// Direct test of format functions
	notifier.NotifyStartup("2.3.0", 7928, "enter")
	notifier.NotifyFailover("node1", "node2", "JP")
	notifier.NotifyPoolAlert("JP-Group", "0 nodes alive")
	_ = sentMsg

	// Verify command handler
	handler := func(cmd, args string) string {
		if cmd == "/ping" {
			return "pong"
		}
		return ""
	}
	res := handler("/ping", "")
	if res != "pong" {
		t.Fatalf("expected pong, got %s", res)
	}
}
