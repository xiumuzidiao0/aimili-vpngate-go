package vpn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
)

type Manager struct {
	cfg      *config.Config
	pool     *nodes.NodePool
	runner   *OpenVPNRunner

	mu            sync.RWMutex
	epoch         uint64
	cancelFunc    context.CancelFunc
	status        ConnectionStatus
	activeNode    *nodes.Node
	connectedAt   time.Time
	lastMessage   string
	reconnects    int
	isConnecting  bool
}

func NewManager(cfg *config.Config, pool *nodes.NodePool) *Manager {
	return &Manager{
		cfg:         cfg,
		pool:        pool,
		runner:      NewOpenVPNRunner(cfg),
		status:      StatusDisconnected,
		lastMessage: "VPN 服务已就绪，未连接",
	}
}

func (m *Manager) Snapshot() StateSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime int64
	if m.status == StatusConnected && !m.connectedAt.IsZero() {
		uptime = int64(time.Since(m.connectedAt).Seconds())
	}

	var activeID string
	if m.activeNode != nil {
		activeID = m.activeNode.ID
	}

	return StateSnapshot{
		Status:         m.status,
		StatusText:     string(m.status),
		ActiveNodeID:   activeID,
		ActiveNode:     m.activeNode,
		TunnelReady:    m.status == StatusConnected,
		ProxyReady:     true,
		ConnectedAt:    m.connectedAt,
		UptimeSeconds:  uptime,
		LastMessage:    m.lastMessage,
		ReconnectCount: m.reconnects,
	}
}

func (m *Manager) Connect(target *nodes.Node) error {
	m.mu.Lock()
	m.epoch++
	currentEpoch := m.epoch

	// Cancel prior attempt if any
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	m.runner.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel
	m.activeNode = target
	m.status = StatusConnecting
	m.isConnecting = true
	m.lastMessage = fmt.Sprintf("正在发起对节点 %s (%s) 的连接...", target.ID, target.CountryShort)
	m.mu.Unlock()

	stats.LogInfo("VPN", "开始连接节点: %s (%s %s)", target.ID, target.CountryShort, target.HostName)

	// Prepare config & credentials
	if err := m.runner.PrepareFiles(target); err != nil {
		m.mu.Lock()
		if m.epoch == currentEpoch {
			m.status = StatusFailed
			m.isConnecting = false
			m.lastMessage = fmt.Sprintf("配置文件准备失败: %v", err)
		}
		m.mu.Unlock()
		stats.LogError("VPN", "配置准备失败: %v", err)
		return err
	}

	listener := func(evt ProcessEvent, msg string) {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.epoch != currentEpoch {
			return // Stale event
		}

		switch evt {
		case EventConnected:
			m.status = StatusConnected
			m.isConnecting = false
			m.connectedAt = time.Now()
			m.lastMessage = "VPN 隧道已成功连通并就绪"
			stats.LogInfo("VPN", "节点 [%s] 连通就绪！", target.ID)

		case EventAuthFail:
			m.status = StatusFailed
			m.isConnecting = false
			m.lastMessage = "认证失败，将节点列入黑名单"
			m.pool.Blacklist().Mark(target, "认证失败", m.cfg.InvalidBackoff)
			stats.LogWarn("VPN", "节点 [%s] 认证失败，已列入黑名单", target.ID)
			go m.TriggerAutoFailover()

		case EventError:
			stats.LogWarn("VPN", "节点 [%s] 出现异常: %s", target.ID, msg)

		case EventExited:
			if m.status == StatusConnected || m.status == StatusConnecting {
				m.status = StatusFailed
				m.isConnecting = false
				m.lastMessage = "OpenVPN 进程异常退出"
				m.pool.Blacklist().Mark(target, "进程异常断开", m.cfg.InvalidBackoff)
				stats.LogWarn("VPN", "节点 [%s] 意外中断，准备自动故障转移", target.ID)
				go m.TriggerAutoFailover()
			}
		}
	}

	if err := m.runner.Start(ctx, listener); err != nil {
		m.mu.Lock()
		if m.epoch == currentEpoch {
			m.status = StatusFailed
			m.isConnecting = false
			m.lastMessage = fmt.Sprintf("启动 OpenVPN 失败: %v", err)
		}
		m.mu.Unlock()
		return err
	}

	return nil
}

func (m *Manager) Disconnect(reason string) {
	m.mu.Lock()
	m.epoch++
	if m.cancelFunc != nil {
		m.cancelFunc()
		m.cancelFunc = nil
	}
	m.runner.Stop()
	m.status = StatusDisconnected
	m.activeNode = nil
	m.isConnecting = false
	m.lastMessage = fmt.Sprintf("已断开连接: %s", reason)
	m.mu.Unlock()

	stats.LogInfo("VPN", "VPN 已主动断开: %s", reason)
}

func (m *Manager) TriggerAutoFailover() {
	m.mu.Lock()
	if m.isConnecting {
		m.mu.Unlock()
		return
	}
	m.reconnects++
	m.status = StatusReconnecting
	m.lastMessage = "检测到隧道中断，正在挑选最优新节点自动切换..."
	m.mu.Unlock()

	time.Sleep(2 * time.Second)

	best := m.pool.SelectBest()
	if best == nil {
		stats.LogWarn("VPN", "未找到可用节点，尝试刷新节点池...")
		_ = m.pool.Refresh(context.Background())
		best = m.pool.SelectBest()
	}

	if best != nil {
		stats.LogInfo("VPN", "自动切换至新节点: %s (%s)", best.ID, best.CountryShort)
		_ = m.Connect(best)
	} else {
		stats.LogError("VPN", "自动故障转移失败: 无任何候选节点可用")
	}
}

func (m *Manager) StartHealthChecker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.RLock()
				status := m.status
				m.mu.RUnlock()

				if status != StatusConnected {
					continue
				}

				// Check external connectivity
				if !CheckExternalConnectivity(5 * time.Second) {
					stats.LogWarn("Health", "心跳检测未通过：外部网络连通性中断，触发重试...")
					time.Sleep(2 * time.Second)
					if !CheckExternalConnectivity(5 * time.Second) {
						stats.LogError("Health", "外部网络持续不通，判定当前 VPN 节点失效")
						go m.TriggerAutoFailover()
					}
				}
			}
		}
	}()
}
