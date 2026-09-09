package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/tunnel"
)

type MultiPortManager struct {
	cfg       *config.Config
	tunnelPool *tunnel.Pool
	scheduler TunnelSelector

	mu        sync.RWMutex
	listeners map[int]*PortListener
	rules     []PortRule
	rulesPath string
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewMultiPortManager(cfg *config.Config, tp *tunnel.Pool) *MultiPortManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &MultiPortManager{
		cfg:        cfg,
		tunnelPool: tp,
		scheduler:  NewScheduler(tp),
		listeners:  make(map[int]*PortListener),
		rulesPath:  filepath.Join(cfg.DataDir, "port_rules.json"),
		ctx:        ctx,
		cancel:     cancel,
	}
	m.loadRules()
	return m
}

func (m *MultiPortManager) loadRules() {
	data, err := os.ReadFile(m.rulesPath)
	if err == nil {
		var list []PortRule
		if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
			m.rules = list
			return
		}
	}

	// Default: 1 primary port rule matching cfg.ProxyPort
	primaryPort := m.cfg.ProxyPort
	if primaryPort <= 0 {
		primaryPort = 7928
	}

	m.rules = []PortRule{
		{
			Port:            primaryPort,
			Enabled:         true,
			BoundTunnelIDs:  nil, // all healthy tunnels
			Policy:          PolicyRoundRobin,
			IntervalSeconds: 300,
			AuthMode:        "default_web",
		},
	}
	m.saveRulesLocked()
}

func (m *MultiPortManager) saveRulesLocked() {
	data, err := json.MarshalIndent(m.rules, "", "  ")
	if err != nil {
		return
	}
	tmp := m.rulesPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err == nil {
		_ = os.Rename(tmp, m.rulesPath)
	}
}

func (m *MultiPortManager) StartAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rule := range m.rules {
		if rule.Enabled {
			m.startListenerLocked(rule)
		}
	}
}

func (m *MultiPortManager) startListenerLocked(rule PortRule) {
	if _, exists := m.listeners[rule.Port]; exists {
		return
	}

	listener := NewPortListener(rule, m.cfg, m.scheduler)
	m.listeners[rule.Port] = listener
	go func() {
		if err := listener.Start(m.ctx); err != nil {
			stats.LogWarn("ProxyManager", "启动端口 %d 监听失败: %v", rule.Port, err)
		}
	}()
}

func (m *MultiPortManager) stopListenerLocked(port int) {
	if l, exists := m.listeners[port]; exists {
		l.Close()
		delete(m.listeners, port)
	}
}

func (m *MultiPortManager) ApplyRules(newRules []PortRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check duplicates
	portMap := make(map[int]bool)
	for _, r := range newRules {
		if r.Port <= 0 || r.Port > 65535 {
			return fmt.Errorf("非法端口号: %d", r.Port)
		}
		if r.Port == m.cfg.UIPort {
			return fmt.Errorf("代理端口不能与 Web 控制台端口 (%d) 冲突", m.cfg.UIPort)
		}
		if portMap[r.Port] {
			return fmt.Errorf("存在重复配置的代理端口: %d", r.Port)
		}
		portMap[r.Port] = true
	}

	// Stop listeners for removed ports
	for port := range m.listeners {
		if !portMap[port] {
			m.stopListenerLocked(port)
		}
	}

	// Update or restart listeners
	for _, rule := range newRules {
		if currentListener, exists := m.listeners[rule.Port]; exists {
			// Restart if rule parameters changed or disabled
			currentListener.Close()
			delete(m.listeners, rule.Port)
		}

		if rule.Enabled {
			m.startListenerLocked(rule)
		}
	}

	m.rules = newRules
	m.saveRulesLocked()
	stats.LogInfo("ProxyManager", "已成功应用并更新 %d 条多端口代理分流规则", len(newRules))
	return nil
}

func (m *MultiPortManager) GetRules() []PortRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]PortRule, len(m.rules))
	copy(res, m.rules)
	return res
}

func (m *MultiPortManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancel()
	for _, l := range m.listeners {
		l.Close()
	}
	m.listeners = make(map[int]*PortListener)
}
