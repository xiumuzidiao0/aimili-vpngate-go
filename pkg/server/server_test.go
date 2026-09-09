package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/vpn"
)

func TestServerAPI(t *testing.T) {
	cfg := &config.Config{
		UIPath:     "testadmin",
		UIUsername: "admin",
		UIPassword: "password123",
		DataDir:    t.TempDir(),
	}

	pool := nodes.NewNodePool(cfg)
	vpnMgr := vpn.NewManager(cfg, pool)
	srv := NewServer(cfg, pool, vpnMgr)

	// Test Status endpoint handler directly
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse status json: %v", err)
	}
	if resp.AdminPath != "testadmin" {
		t.Fatalf("expected admin path testadmin, got %s", resp.AdminPath)
	}

	// Test Metrics endpoint
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	srv.handleMetrics(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", wMetrics.Code)
	}
	metricsBody := wMetrics.Body.String()
	if !strings.Contains(metricsBody, "aimili_traffic_upload_bytes_total") {
		t.Fatalf("metrics body missing upload counter: %s", metricsBody)
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		UIUsername: "myuser",
		UIPassword: "mypassword",
	}
	mw := NewMiddleware(cfg)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := mw.BasicAuth(dummyHandler)

	// 1. Unauthenticated request -> 401
	req1 := httptest.NewRequest("GET", "/", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", w1.Code)
	}

	// 2. Authenticated request -> 200
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.SetBasicAuth("myuser", "mypassword")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w2.Code)
	}
}
