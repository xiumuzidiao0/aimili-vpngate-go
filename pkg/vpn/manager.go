package vpn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/tunnel"
)

type Manager struct {
	cfg        *config.Config
	pool       *nodes.NodePool
	tunnelPool *tunnel.Pool

	mu           sync.RWMutex
	epoch        uint64
	primaryID    string
	status       ConnectionStatus
	activeNode   *nodes.Node
	connectedAt  time.Time
	lastMessage  string
	reconnects   int
	isConnecting bool
}

func NewManager(cfg *config.Config, pool *nodes.NodePool, tp *tunnel.Pool) *Manager {
	return &Manager{
		cfg:         cfg,
		pool:        pool,
		tunnelPool:  tp,
		status:      StatusDisconnected,
		lastMessage: "VPN 服务已就绪，未连接",
	}
}

func (m *Manager) Snapshot() StateSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := m.status
	lastMsg := m.lastMessage
	activeNode := m.activeNode
	connectedAt := m.connectedAt

	if m.primaryID != "" && m.tunnelPool != nil {
		if t := m.tunnelPool.GetTunnel(m.primaryID); t != nil {
			if t.Status == tunnel.StatusConnected {
				status = StatusConnected
			} else if t.Status == tunnel.StatusConnecting {
				status = StatusConnecting
			} else if t.Status == tunnel.StatusFailed {
				status = StatusFailed
			}
			activeNode = t.Node
			connectedAt = t.ConnectedAt
			if t.Message != "" {
				lastMsg = t.Message
			}
		}
	}

	var uptime int64
	if status == StatusConnected && !connectedAt.IsZero() {
		uptime = int64(time.Since(connectedAt).Seconds())
	}

	var activeID string
	if activeNode != nil {
		activeID = activeNode.ID
	}

	return StateSnapshot{
		Status:         status,
		StatusText:     string(status),
		ActiveNodeID:   activeID,
		ActiveNode:     activeNode,
		TunnelReady:    status == StatusConnected,
		ProxyReady:     true,
		ConnectedAt:    connectedAt,
		UptimeSeconds:  uptime,
		LastMessage:    lastMsg,
		ReconnectCount: m.reconnects,
	}
}

func (m *Manager) Connect(target *nodes.Node) error {
	m.mu.Lock()
	m.epoch++
	currentEpoch := m.epoch

	// Stop previous primary tunnel if running
	if m.primaryID != "" && m.tunnelPool != nil {
		_ = m.tunnelPool.StopTunnel(m.primaryID)
		m.primaryID = ""
	}

	m.activeNode = target
	m.status = StatusConnecting
	m.isConnecting = true
	m.lastMessage = fmt.Sprintf("正在发起对节点 %s (%s) 的连接...", target.ID, target.CountryShort)
	m.mu.Unlock()

	stats.LogInfo("VPN", "开始连接主节点: %s (%s %s)", target.ID, target.CountryShort, target.HostName)

	if m.tunnelPool == nil {
		m.mu.Lock()
		m.status = StatusFailed
		m.isConnecting = false
		m.lastMessage = "隧道池未初始化"
		m.mu.Unlock()
		return fmt.Errorf("tunnel pool is nil")
	}

	tun, err := m.tunnelPool.StartTunnel(target)
	if err != nil {
		m.mu.Lock()
		if m.epoch == currentEpoch {
			m.status = StatusFailed
			m.isConnecting = false
			m.lastMessage = fmt.Sprintf("启动主隧道失败: %v", err)
		}
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	if m.epoch == currentEpoch {
		m.primaryID = tun.ID
	}
	m.mu.Unlock()

	// Monitor until ready or failed
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()

		timeout := time.After(20 * time.Second)

		for {
			select {
			case <-timeout:
				m.mu.Lock()
				if m.epoch == currentEpoch && m.status == StatusConnecting {
					m.status = StatusFailed
					m.isConnecting = false
					m.lastMessage = "连接超时"
				}
				m.mu.Unlock()
				return
			case <-ticker.C:
				m.mu.RLock()
				curEpoch := m.epoch
				pID := m.primaryID
				m.mu.RUnlock()

				if curEpoch != currentEpoch {
					return
				}

				t := m.tunnelPool.GetTunnel(pID)
				if t == nil {
					return
				}

				status := t.GetStatus()
				if status == tunnel.StatusConnected {
					m.mu.Lock()
					if m.epoch == currentEpoch {
						m.status = StatusConnected
						m.isConnecting = false
						m.connectedAt = time.Now()
						m.lastMessage = "VPN 隧道已成功连通并就绪"
					}
					m.mu.Unlock()
					return
				} else if status == tunnel.StatusFailed {
					m.mu.Lock()
					if m.epoch == currentEpoch {
						m.status = StatusFailed
						m.isConnecting = false
						m.lastMessage = t.Message
					}
					m.mu.Unlock()
					go m.TriggerAutoFailover()
					return
				}
			}
		}
	}()

	return nil
}

func (m *Manager) Disconnect(reason string) {
	m.mu.Lock()
	m.epoch++
	if m.primaryID != "" && m.tunnelPool != nil {
		_ = m.tunnelPool.StopTunnel(m.primaryID)
		m.primaryID = ""
	}
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
		stats.LogInfo("VPN", "自动切换至新主节点: %s (%s)", best.ID, best.CountryShort)
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

func (m *Manager) StartAutoRotator(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		var lastRotated time.Time

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				settings := m.cfg.GetSettings()
				if settings.AutoRotateMinutes <= 0 {
					continue
				}

				rotateInterval := time.Duration(settings.AutoRotateMinutes) * time.Minute
				if !lastRotated.IsZero() && time.Since(lastRotated) < rotateInterval {
					continue
				}

				m.mu.RLock()
				status := m.status
				currID := ""
				if m.activeNode != nil {
					currID = m.activeNode.ID
				}
				m.mu.RUnlock()

				if status != StatusConnected {
					continue
				}

				best := m.pool.SelectBestWithFilter(settings.AutoRotateIPType, settings.DiscoveryCountries, true)
				if best != nil && best.ID != currID {
					stats.LogInfo("Rotator", "触发定时自动轮换策略 (周期: %d分钟)，正在平滑切换至新节点: %s (%s)...",
						settings.AutoRotateMinutes, best.ID, best.CountryShort)
					_ = m.Connect(best)
					lastRotated = time.Now()
				}
			}
		}
	}()
}
