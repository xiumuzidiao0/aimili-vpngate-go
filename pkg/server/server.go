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
	"aimili-vpngate-go/pkg/notify"
	"aimili-vpngate-go/pkg/proxy"
	"aimili-vpngate-go/pkg/singbox"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/tunnel"
	"aimili-vpngate-go/pkg/vpn"
	"aimili-vpngate-go/web"
)

type Server struct {
	cfg           *config.Config
	pool          *nodes.NodePool
	vpn           *vpn.Manager
	tunnelPool    *tunnel.Pool
	dynamicMgr    *tunnel.DynamicGroupManager
	portMgr       *proxy.MultiPortManager
	notifier      *notify.TelegramNotifier
	singboxClient *singbox.Client
	httpServer    *http.Server
	sseHub        *SSEHub
	mu            sync.Mutex
	listener      net.Listener
}

func NewServer(cfg *config.Config, pool *nodes.NodePool, vpnMgr *vpn.Manager, tp *tunnel.Pool, dm *tunnel.DynamicGroupManager, pm *proxy.MultiPortManager, tn *notify.TelegramNotifier) *Server {
	s := &Server{
		cfg:           cfg,
		pool:          pool,
		vpn:           vpnMgr,
		tunnelPool:    tp,
		dynamicMgr:    dm,
		portMgr:       pm,
		notifier:      tn,
		singboxClient: singbox.NewClient(),
	}
	s.sseHub = NewSSEHub(s)
	return s
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/nodes", s.handleNodes)
	mux.HandleFunc("POST /api/nodes/probe", s.handleProbeNodes)
	mux.HandleFunc("POST /api/nodes/favorite", s.handleToggleFavorite)
	mux.HandleFunc("POST /api/connect", s.handleConnect)
	mux.HandleFunc("POST /api/disconnect", s.handleDisconnect)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("GET /api/blacklist", s.handleBlacklist)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("POST /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/events", s.sseHub.HandleEvents)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Multi-Tunnel & Multi-Port Matrix APIs
	mux.HandleFunc("GET /api/tunnels", s.handleListTunnels)
	mux.HandleFunc("POST /api/tunnels/start", s.handleStartTunnel)
	mux.HandleFunc("POST /api/tunnels/stop", s.handleStopTunnel)
	mux.HandleFunc("GET /api/proxy/ports", s.handleGetPortRules)
	mux.HandleFunc("POST /api/proxy/ports", s.handleSetPortRules)

	// Dynamic Auto-Managed Tunnel Groups APIs
	mux.HandleFunc("GET /api/tunnel-groups", s.handleListTunnelGroups)
	mux.HandleFunc("POST /api/tunnel-groups", s.handleSaveTunnelGroup)
	mux.HandleFunc("DELETE /api/tunnel-groups", s.handleDeleteTunnelGroup)
	mux.HandleFunc("POST /api/tunnel-groups/evaluate", s.handleEvaluateTunnelGroups)

	// AI & Streaming Unlock APIs
	mux.HandleFunc("GET /api/unlock", s.handleGetUnlockStatus)
	mux.HandleFunc("POST /api/unlock/probe", s.handleProbeTunnelUnlock)

	// Reputation & Telegram Integration APIs
	mux.HandleFunc("GET /api/reputation", s.handleGetReputation)
	mux.HandleFunc("POST /api/telegram/test", s.handleTelegramTest)

	// sing-box Inbound & Outbound Integration APIs
	mux.HandleFunc("GET /api/singbox/overview", s.handleSingBoxOverview)
	mux.HandleFunc("GET /api/singbox/status", s.handleSingBoxStatus)
	mux.HandleFunc("GET /api/singbox/protocols", s.handleSingBoxProtocols)
	mux.HandleFunc("GET /api/singbox/nodes", s.handleSingBoxListNodes)
	mux.HandleFunc("POST /api/singbox/nodes", s.handleSingBoxAddNode)
	mux.HandleFunc("POST /api/singbox/nodes/outbound", s.handleSingBoxSetOutbound)
	mux.HandleFunc("DELETE /api/singbox/nodes", s.handleSingBoxDeleteNode)
	mux.HandleFunc("GET /api/singbox/subscription", s.handleSingBoxGetSub)
	mux.HandleFunc("POST /api/singbox/subscription/sync", s.handleSingBoxSyncSub)
	mux.HandleFunc("POST /api/singbox/subscription/init", s.handleSingBoxInitSub)

	// Static UI file server
	fileServer := http.FileServer(web.GetFileSystem())
	mux.Handle("/", fileServer)

	// Apply Middlewares
	mw := NewMiddleware(s.cfg)
	handler := mw.BasicAuth(mux)
	handler = mw.SecretPathGuard(handler)
	handler = mw.SecurityHeaders(handler)

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
