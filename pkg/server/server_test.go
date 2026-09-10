package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/proxy"
	"aimili-vpngate-go/pkg/tunnel"
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
	tunnelPool := tunnel.NewPool(cfg, pool)
	vpnMgr := vpn.NewManager(cfg, pool, tunnelPool)
	dynamicMgr := tunnel.NewDynamicGroupManager(cfg, tunnelPool, pool)
	portMgr := proxy.NewMultiPortManager(cfg, tunnelPool, dynamicMgr)
	srv := NewServer(cfg, pool, vpnMgr, tunnelPool, dynamicMgr, portMgr, nil)

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

	// Test Get Settings
	reqSettings := httptest.NewRequest("GET", "/api/settings", nil)
	wSettings := httptest.NewRecorder()
	srv.handleGetSettings(wSettings, reqSettings)
	if wSettings.Code != http.StatusOK {
		t.Fatalf("expected settings 200, got %d", wSettings.Code)
	}

	// Test Update Settings
	updateBody := `{"ui_username":"newadmin","ui_path":"newsecret","ui_port":8888,"proxy_port":7999}`
	reqUpdate := httptest.NewRequest("POST", "/api/settings", strings.NewReader(updateBody))
	wUpdate := httptest.NewRecorder()
	srv.handleUpdateSettings(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected update settings 200, got %d", wUpdate.Code)
	}
	if srv.cfg.GetSettings().UIUsername != "newadmin" {
		t.Fatalf("expected newadmin, got %s", srv.cfg.GetSettings().UIUsername)
	}

	// Test Save Dynamic Tunnel Group
	groupJSON := `{"id":"test-dg","name":"日本Top3住宅组","enabled":true,"country":"JP","ip_type":"residential","sort_by":"latency","target_count":3,"interval_minutes":15}`
	reqGroup := httptest.NewRequest("POST", "/api/tunnel-groups", strings.NewReader(groupJSON))
	wGroup := httptest.NewRecorder()
	srv.handleSaveTunnelGroup(wGroup, reqGroup)
	if wGroup.Code != http.StatusOK {
		t.Fatalf("expected save group 200, got %d", wGroup.Code)
	}

	// Test List Dynamic Groups
	reqListGroups := httptest.NewRequest("GET", "/api/tunnel-groups", nil)
	wListGroups := httptest.NewRecorder()
	srv.handleListTunnelGroups(wListGroups, reqListGroups)
	if wListGroups.Code != http.StatusOK {
		t.Fatalf("expected list groups 200, got %d", wListGroups.Code)
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
