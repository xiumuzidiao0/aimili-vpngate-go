package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/stats"
)

type Gateway struct {
	cfg      *config.Config
	auth     *Authenticator
	listener net.Listener
	sem      chan struct{}
	mu       sync.Mutex
	closed   bool
}

func NewGateway(cfg *config.Config) *Gateway {
	auth := NewAuthenticator(cfg.ProxyUser, cfg.ProxyPass)
	maxConn := cfg.ProxyMaxConnections
	if maxConn <= 0 {
		maxConn = 512
	}

	return &Gateway{
		cfg:  cfg,
		auth: auth,
		sem:  make(chan struct{}, maxConn),
	}
}

func (g *Gateway) Start(ctx context.Context) error {
	addr := net.JoinHostPort(g.cfg.ProxyHost, fmt.Sprintf("%d", g.cfg.ProxyPort))
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		// Fallback for IPv6 [::] failure
		if g.cfg.ProxyHost == "::" {
			fallbackAddr := net.JoinHostPort("0.0.0.0", fmt.Sprintf("%d", g.cfg.ProxyPort))
			ln, err = lc.Listen(ctx, "tcp", fallbackAddr)
		}
		if err != nil {
			return fmt.Errorf("proxy gateway failed to listen on %s: %w", addr, err)
		}
	}

	g.mu.Lock()
	g.listener = ln
	g.mu.Unlock()

	stats.LogInfo("Gateway", "本地统一代理网关已就绪，正在监听: %s (支持 SOCKS5 / HTTP / HTTPS CONNECT)", ln.Addr().String())

	go func() {
		<-ctx.Done()
		g.Close()
	}()

	for {
		client, err := ln.Accept()
		if err != nil {
			g.mu.Lock()
			isClosed := g.closed
			g.mu.Unlock()
			if isClosed {
				return nil
			}
			stats.LogWarn("Gateway", "网关接受连接失败: %v", err)
			continue
		}

		select {
		case g.sem <- struct{}{}:
			stats.GetTrafficTracker().IncConn()
			go func(c net.Conn) {
				defer func() {
					<-g.sem
					stats.GetTrafficTracker().DecConn()
				}()
				g.dispatch(c)
			}(client)
		default:
			stats.LogWarn("Gateway", "代理并发连接数已达上限 (%d)，丢弃新连接: %s", g.cfg.ProxyMaxConnections, client.RemoteAddr())
			_ = client.Close()
		}
	}
}

func (g *Gateway) dispatch(client net.Conn) {
	_ = client.SetDeadline(time.Now().Add(60 * time.Second))
	br := bufio.NewReader(client)

	// Peek at the first byte
	firstByte, err := br.Peek(1)
	if err != nil {
		_ = client.Close()
		return
	}

	// Reset deadline for active proxying
	_ = client.SetDeadline(time.Time{})

	bConn := &bufferedConn{
		Conn: client,
		br:   br,
	}

	if firstByte[0] == socks5Version {
		// SOCKS5 protocol
		if err := handleSocks5(bConn, g.auth); err != nil {
			// Quiet on normal disconnect
		}
	} else {
		// HTTP / HTTPS CONNECT protocol
		if err := handleHTTP(bConn, br, g.auth); err != nil {
			// Quiet on normal disconnect
		}
	}
}

func (g *Gateway) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.closed {
		g.closed = true
		if g.listener != nil {
			_ = g.listener.Close()
		}
		stats.LogInfo("Gateway", "代理网关已优雅关闭")
	}
}
