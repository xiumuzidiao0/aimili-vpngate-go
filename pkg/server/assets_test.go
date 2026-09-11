package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/web"
)

func TestStaticAssetsUnderSecretPath(t *testing.T) {
	cfg := &config.Config{
		UIPath:     "enter",
		UIUsername: "admin",
		UIPassword: "password123",
	}
	mw := NewMiddleware(cfg)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(web.GetFileSystem()))

	handler := mw.SecretPathGuard(mw.BasicAuth(mux))

	// 1. /enter/ should return index.html
	reqIndex := httptest.NewRequest("GET", "/enter/", nil)
	reqIndex.SetBasicAuth("admin", "password123")
	wIndex := httptest.NewRecorder()
	handler.ServeHTTP(wIndex, reqIndex)
	if wIndex.Code != http.StatusOK || !strings.Contains(wIndex.Body.String(), "<!DOCTYPE html>") {
		t.Fatalf("expected 200 for index.html, got %d", wIndex.Code)
	}

	// 2. /enter/styles.css should return styles.css
	reqCSS := httptest.NewRequest("GET", "/enter/styles.css", nil)
	reqCSS.SetBasicAuth("admin", "password123")
	wCSS := httptest.NewRecorder()
	handler.ServeHTTP(wCSS, reqCSS)
	if wCSS.Code != http.StatusOK || !strings.Contains(wCSS.Body.String(), "--bg-base") {
		t.Fatalf("expected 200 for styles.css, got %d", wCSS.Code)
	}

	// 3. /enter/app.js should return app.js
	reqJS := httptest.NewRequest("GET", "/enter/app.js", nil)
	reqJS.SetBasicAuth("admin", "password123")
	wJS := httptest.NewRecorder()
	handler.ServeHTTP(wJS, reqJS)
	if wJS.Code != http.StatusOK || !strings.Contains(wJS.Body.String(), "escapeHtml") {
		t.Fatalf("expected 200 for app.js, got %d", wJS.Code)
	}

	// 4. /enter/favicon.svg should return favicon.svg
	reqFav := httptest.NewRequest("GET", "/enter/favicon.svg", nil)
	reqFav.SetBasicAuth("admin", "password123")
	wFav := httptest.NewRecorder()
	handler.ServeHTTP(wFav, reqFav)
	if wFav.Code != http.StatusOK || !strings.Contains(wFav.Body.String(), "<svg") {
		t.Fatalf("expected 200 for favicon.svg, got %d", wFav.Code)
	}
}
