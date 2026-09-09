package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"aimili-vpngate-go/pkg/config"
)

type Middleware struct {
	cfg *config.Config
}

func NewMiddleware(cfg *config.Config) *Middleware {
	return &Middleware{cfg: cfg}
}

func (m *Middleware) BasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If both username and password are empty, auth is disabled
		if m.cfg.UIUsername == "" && m.cfg.UIPassword == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(m.cfg.UIUsername)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(m.cfg.UIPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="AimiliVPN Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) SecretPathGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.Trim(m.cfg.UIPath, "/")
		if secret == "" {
			next.ServeHTTP(w, r)
			return
		}

		path := strings.Trim(r.URL.Path, "/")
		// Allow API endpoints directly if already inside or if authorized
		if strings.HasPrefix(path, "api/") || path == "api" || path == "metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if path matches secret or starts with secret/
		if path == secret || strings.HasPrefix(path, secret+"/") {
			// Strip secret prefix for serving frontend files
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = strings.TrimPrefix(path, secret)
			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}
			next.ServeHTTP(w, r2)
			return
		}

		// Reject unauthorized direct access to root
		http.Error(w, "404 Not Found (Invalid Admin Path)", http.StatusNotFound)
	})
}
