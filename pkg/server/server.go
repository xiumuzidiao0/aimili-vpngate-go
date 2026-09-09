package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/vpn"
	"aimili-vpngate-go/web"
)

type Server struct {
	cfg        *config.Config
	pool       *nodes.NodePool
	vpn        *vpn.Manager
	httpServer *http.Server
	sseHub     *SSEHub
	mu         sync.Mutex
	listener   net.Listener
}

func NewServer(cfg *config.Config, pool *nodes.NodePool, vpnMgr *vpn.Manager) *Server {
	s := &Server{
		cfg:  cfg,
		pool: pool,
		vpn:  vpnMgr,
	}
	s.sseHub = NewSSEHub(s)
	return s
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/nodes", s.handleNodes)
	mux.HandleFunc("POST /api/connect", s.handleConnect)
	mux.HandleFunc("POST /api/disconnect", s.handleDisconnect)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("GET /api/blacklist", s.handleBlacklist)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("POST /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/events", s.sseHub.HandleEvents)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Static UI file server
	fileServer := http.FileServer(web.GetFileSystem())
	mux.Handle("/", fileServer)

	// Apply Middlewares
	mw := NewMiddleware(s.cfg)
	handler := mw.BasicAuth(mux)
	handler = mw.SecretPathGuard(handler)

	addr := net.JoinHostPort(s.cfg.UIHost, fmt.Sprintf("%d", s.cfg.UIPort))
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		if s.cfg.UIHost == "::" {
			fallbackAddr := net.JoinHostPort("0.0.0.0", fmt.Sprintf("%d", s.cfg.UIPort))
			ln, err = lc.Listen(ctx, "tcp", fallbackAddr)
		}
		if err != nil {
			return fmt.Errorf("web server failed to listen on %s: %w", addr, err)
		}
	}

	s.mu.Lock()
	s.listener = ln
	s.httpServer = &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 allows SSE long streaming
	}
	s.mu.Unlock()

	adminURL := fmt.Sprintf("http://%s/%s", ln.Addr().String(), strings.Trim(s.cfg.UIPath, "/"))
	stats.LogInfo("Server", "Web 控制台已启动，访问入口: %s (用户名: %s)", adminURL, s.cfg.UIUsername)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}
