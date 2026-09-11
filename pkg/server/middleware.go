package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aimili-vpngate-go/pkg/config"
)

type contextKey string

const secretPathVerifiedKey contextKey = "secret_path_verified"

const maxRequestBodyBytes = 1 << 20

type Middleware struct {
	cfg *config.Config
}

func NewMiddleware(cfg *config.Config) *Middleware {
	return &Middleware{cfg: cfg}
}

// SecurityHeaders injects defensive HTTP headers on all responses
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) BasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.IsUIAuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Allow subscription clients (Clash / Mihomo / Shadowrocket) to fetch subscription
		// without HTTP Basic Auth if accessed via verified secret path or with a valid token.
		if strings.HasPrefix(r.URL.Path, "/api/singbox/subscription") {
			if v, ok := r.Context().Value(secretPathVerifiedKey).(bool); ok && v {
				next.ServeHTTP(w, r)
				return
			}
			token := r.URL.Query().Get("token")
			settings := m.cfg.GetSettings()
			secret := strings.Trim(settings.UIPath, "/")
			if token != "" && (token == secret || token == settings.UIPassword) {
				next.ServeHTTP(w, r)
				return
			}
		}

		user, pass, ok := r.BasicAuth()
		if !ok || !m.cfg.VerifyUICredentials(user, pass) {
			// Anti brute-force delay on invalid attempt
			time.Sleep(300 * time.Millisecond)
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted Access"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) SecretPathGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := m.cfg.GetSettings()
		secret := strings.Trim(settings.UIPath, "/")
		if secret == "" {
			next.ServeHTTP(w, r)
			return
		}

		reqPath := r.URL.Path

		// Direct subscription fetch with valid token bypasses secret path prefix
		token := r.URL.Query().Get("token")
		if strings.HasPrefix(reqPath, "/api/singbox/subscription") && token != "" && (token == secret || token == settings.UIPassword) {
			ctx := context.WithValue(r.Context(), secretPathVerifiedKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Direct /api/ or /metrics access without secret prefix:
		// Only allow if caller already holds valid authentication credentials.
		// If unauthenticated, stealthily return 404 to avoid revealing that this API exists to scanners.
		if strings.HasPrefix(reqPath, "/api/") || reqPath == "/api" || reqPath == "/metrics" {
			if m.cfg.IsUIAuthEnabled() {
				user, pass, ok := r.BasicAuth()
				if !ok || !m.cfg.VerifyUICredentials(user, pass) {
					http.NotFound(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
			return
		}

		// Exact match to /secret -> redirect to /secret/
		if reqPath == "/"+secret {
			http.Redirect(w, r, "/"+secret+"/", http.StatusFound)
			return
		}

		// Scoped under /secret/
		prefix := "/" + secret + "/"
		if strings.HasPrefix(reqPath, prefix) {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/" + strings.TrimPrefix(reqPath, prefix)
			ctx := context.WithValue(r2.Context(), secretPathVerifiedKey, true)
			*r2 = *r2.WithContext(ctx)
			next.ServeHTTP(w, r2)
			return
		}

		// Reject all unauthorized access with generic 404
		http.NotFound(w, r)
	})
}
