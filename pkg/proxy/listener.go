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

type PortRule struct {
	Port            int        `json:"port"`
	Enabled         bool       `json:"enabled"`
	BoundTunnelIDs  []string   `json:"bound_tunnel_ids"`
	BoundGroupIDs   []string   `json:"bound_group_ids"` // Dynamic group IDs
	Policy          PortPolicy `json:"policy"`
	IntervalSeconds int        `json:"interval_seconds"`
	AuthMode        string     `json:"auth_mode"` // "default_web", "custom", "none"
	AuthUser        string     `json:"auth_user"`
	AuthPass        string     `json:"auth_pass"`
}

type PortListener struct {
	rule      PortRule
	cfg       *config.Config
	scheduler TunnelSelector
	listener  net.Listener
	sem       chan struct{}
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
}

func NewPortListener(rule PortRule, cfg *config.Config, scheduler TunnelSelector) *PortListener {
	maxConn := cfg.ProxyMaxConnections
	if maxConn <= 0 {
		maxConn = 512
	}

	return &PortListener{
		rule:      rule,
		cfg:       cfg,
		scheduler: scheduler,
		sem:       make(chan struct{}, maxConn),
	}
}

func (l *PortListener) getAuthenticator() *Authenticator {
	mode := l.rule.AuthMode
	if mode == "none" {
		return NewAuthenticator("", "") // Disabled
	}
	if mode == "custom" {
		return NewAuthenticator(l.rule.AuthUser, l.rule.AuthPass)
	}
	// Default: follow Web credentials
	return NewAuthenticator(l.cfg.GetSettings().UIUsername, l.cfg.GetSettings().UIPassword)
}

func (l *PortListener) Start(ctx context.Context) error {
	listenCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel

	addr := fmt.Sprintf(":%d", l.rule.Port)
	lc := net.ListenConfig{}
	ln, err := lc.Listen(listenCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("端口 %d 监听失败: %w", l.rule.Port, err)
	}

	l.mu.Lock()
	l.listener = ln
	l.closed = false
	l.mu.Unlock()

	stats.LogInfo("Proxy", "代理端口 [%d] 已就绪监听 (策略: %s, 绑定隧道数: %d)",
		l.rule.Port, l.rule.Policy, len(l.rule.BoundTunnelIDs))

	go func() {
		<-listenCtx.Done()
		l.Close()
	}()

	for {
		client, err := ln.Accept()
		if err != nil {
			l.mu.Lock()
			isClosed := l.closed
			l.mu.Unlock()
			if isClosed {
				return nil
			}
			stats.LogWarn("Proxy", "端口 %d 接收连接异常: %v", l.rule.Port, err)
			continue
		}

		select {
		case l.sem <- struct{}{}:
			stats.GetTrafficTracker().IncConn()
			go func(c net.Conn) {
				defer func() {
					<-l.sem
					stats.GetTrafficTracker().DecConn()
				}()
				l.dispatch(c)
			}(client)
		default:
			stats.LogWarn("Proxy", "端口 %d 并发超限，丢弃连接: %s", l.rule.Port, client.RemoteAddr())
			_ = client.Close()
		}
	}
}

func (l *PortListener) dispatch(client net.Conn) {
	_ = client.SetDeadline(time.Now().Add(60 * time.Second))
	br := bufio.NewReader(client)

	firstByte, err := br.Peek(1)
	if err != nil {
		_ = client.Close()
		return
	}
	_ = client.SetDeadline(time.Time{})

	bConn := &bufferedConn{
		Conn: client,
		br:   br,
	}

	auth := l.getAuthenticator()

	// Select destination tunnel device for this connection
	devName := ""
	if l.scheduler != nil {
		if tun := l.scheduler.SelectTunnel(l.rule.Port, l.rule.BoundTunnelIDs, l.rule.BoundGroupIDs, l.rule.Policy, l.rule.IntervalSeconds); tun != nil {
			devName = tun.DevName
		}
	}

	if firstByte[0] == socks5Version {
		_ = handleSocks5(bConn, auth, devName)
	} else {
		_ = handleHTTP(bConn, br, auth, devName)
	}
}

func (l *PortListener) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.closed {
		l.closed = true
		if l.cancel != nil {
			l.cancel()
		}
		if l.listener != nil {
			_ = l.listener.Close()
		}
		stats.LogInfo("Proxy", "代理端口 [%d] 已停止监听", l.rule.Port)
	}
}
