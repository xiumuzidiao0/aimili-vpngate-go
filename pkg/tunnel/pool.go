package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
)

type Pool struct {
	cfg            *config.Config
	mu             sync.RWMutex
	tunnels        map[string]*Tunnel
	usedDevs       map[int]bool
	nodePool       *nodes.NodePool
	unlockDetector *UnlockDetector
	nextIDSeq      int
}

func NewPool(cfg *config.Config, np *nodes.NodePool) *Pool {
	return &Pool{
		cfg:            cfg,
		tunnels:        make(map[string]*Tunnel),
		usedDevs:       make(map[int]bool),
		nodePool:       np,
		unlockDetector: NewUnlockDetector(cfg.DataDir),
	}
}

func (p *Pool) UnlockDetector() *UnlockDetector {
	return p.unlockDetector
}

func (p *Pool) reapDeadTunnelsLocked() {
	for id, t := range p.tunnels {
		t.mu.RLock()
		status := t.Status
		devIndex := t.DevIndex
		authPath := t.authPath
		confPath := t.confPath
		t.mu.RUnlock()

		if status == StatusFailed || status == StatusStopped {
			p.freeDevIndexLocked(devIndex)
			delete(p.tunnels, id)
			go func(a, c string) {
				if a != "" {
					_ = os.Remove(a)
				}
				if c != "" {
					_ = os.Remove(c)
				}
			}(authPath, confPath)
		}
	}
}

func (p *Pool) allocConcurrentDevIndexLocked() int {
	p.reapDeadTunnelsLocked()
	// Concurrent and dynamic tunnels strictly allocate from 1 to 63,
	// keeping 0 exclusively reserved for the primary connection.
	for i := 1; i < 64; i++ {
		if !p.usedDevs[i] {
			p.usedDevs[i] = true
			return i
		}
	}
	return -1
}

func (p *Pool) freeDevIndexLocked(idx int) {
	delete(p.usedDevs, idx)
}

// StartPrimaryTunnel starts or replaces the dedicated Primary Connection tunnel, strictly bound to devIndex 0 (tun0).
func (p *Pool) StartPrimaryTunnel(node *nodes.Node) (*Tunnel, error) {
	p.mu.Lock()
	p.reapDeadTunnelsLocked()

	// 1. If this node is already running on the primary tunnel (tun0), return it
	for _, t := range p.tunnels {
		if t.Node != nil && t.Node.ID == node.ID && t.DevIndex == 0 {
			if t.Status == StatusConnected || t.Status == StatusConnecting {
				p.mu.Unlock()
				return t, nil
			}
		}
	}

	// 2. If any previous tunnel is occupying tun0 (devIndex 0), stop it first
	var prevPrimaryID string
	for id, t := range p.tunnels {
		if t.DevIndex == 0 {
			prevPrimaryID = id
			break
		}
	}
	if prevPrimaryID != "" {
		p.mu.Unlock()
		_ = p.StopTunnel(prevPrimaryID)
		p.mu.Lock()
		p.reapDeadTunnelsLocked()
	}

	// 3. If this node is already running on a concurrent devIndex (> 0), stop that concurrent tunnel
	// so it can be promoted to primary tun0
	var concurrentID string
	for id, t := range p.tunnels {
		if t.Node != nil && t.Node.ID == node.ID {
			concurrentID = id
			break
		}
	}
	if concurrentID != "" {
		p.mu.Unlock()
		_ = p.StopTunnel(concurrentID)
		p.mu.Lock()
		p.reapDeadTunnelsLocked()
	}

	p.usedDevs[0] = true
	return p.startTunnelInternalLocked(node, 0)
}

// StartTunnel starts a concurrent or dynamic group tunnel, strictly allocating from devIndex 1 upwards (tun1, tun2...).
func (p *Pool) StartTunnel(node *nodes.Node) (*Tunnel, error) {
	p.mu.Lock()
	p.reapDeadTunnelsLocked()

	// Check if already connecting or connected to this node
	for _, t := range p.tunnels {
		if t.Node != nil && t.Node.ID == node.ID && (t.Status == StatusConnected || t.Status == StatusConnecting) {
			p.mu.Unlock()
			return t, fmt.Errorf("节点 %s 已经在运行中 (设备: %s)", node.ID, t.DevName)
		}
	}

	devIdx := p.allocConcurrentDevIndexLocked()
	if devIdx < 0 {
		p.mu.Unlock()
		return nil, fmt.Errorf("并发隧道已达上限 (最多63个并发出口)")
	}

	return p.startTunnelInternalLocked(node, devIdx)
}

func (p *Pool) startTunnelInternalLocked(node *nodes.Node, devIdx int) (*Tunnel, error) {
	p.nextIDSeq++
	tunnelID := fmt.Sprintf("tun-%d", p.nextIDSeq)
	devName := fmt.Sprintf("tun%d", devIdx)

	t := &Tunnel{
		ID:       tunnelID,
		DevName:  devName,
		DevIndex: devIdx,
		Node:     node,
		Status:   StatusConnecting,
		Message:  fmt.Sprintf("正在发起对节点 %s 的连接...", node.ID),
	}
	p.tunnels[tunnelID] = t
	p.mu.Unlock()

	stats.LogInfo("TunnelPool", "开始创建新隧道 [%s] -> 设备 %s -> 节点: %s (%s)",
		tunnelID, devName, node.ID, node.CountryShort)

	// Prepare config & credentials
	dir := filepath.Join(p.cfg.DataDir, "tunnels")
	_ = os.MkdirAll(dir, 0700)

	authPath := filepath.Join(dir, fmt.Sprintf("%s_auth.txt", tunnelID))
	authData := fmt.Sprintf("%s\n%s\n", p.cfg.OpenVPNAuthUser, p.cfg.OpenVPNAuthPass)
	if err := os.WriteFile(authPath, []byte(authData), 0600); err != nil {
		p.StopTunnel(tunnelID)
		return nil, fmt.Errorf("写入认证文件失败: %w", err)
	}

	confPath := filepath.Join(dir, fmt.Sprintf("%s.ovpn", tunnelID))
	lines := strings.Split(node.ConfigData, "\n")
	var modified []string

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "auth-user-pass") ||
			strings.HasPrefix(line, "dev ") ||
			strings.HasPrefix(line, "dev-type") ||
			strings.HasPrefix(line, "verb ") ||
			strings.HasPrefix(line, "redirect-gateway") ||
			strings.HasPrefix(line, "route-gateway") ||
			strings.HasPrefix(line, "route ") {
			continue
		}
		modified = append(modified, rawLine)
	}

	modified = append(modified,
		"auth-user-pass "+authPath,
		fmt.Sprintf("dev %s", devName),
		"dev-type tun",
		"verb 3",
		"route-nopull",
		"pull-filter ignore \"redirect-gateway\"",
		"pull-filter ignore \"route-gateway\"",
		"pull-filter ignore \"route \"",
		"nobind",
		"connect-retry 1 2",
		"connect-retry-max 2",
		"resolv-retry 2",
		"connect-timeout 8",
		"ping 5",
		"ping-restart 12",
		"sndbuf 524288",
		"rcvbuf 524288",
		"txqueuelen 1000",
	)

	if err := os.WriteFile(confPath, []byte(strings.Join(modified, "\n")), 0600); err != nil {
		p.StopTunnel(tunnelID)
		return nil, fmt.Errorf("写入配置文件失败: %w", err)
	}

	t.mu.Lock()
	t.authPath = authPath
	t.confPath = confPath

	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFunc = cancel

	cmd := exec.CommandContext(ctx, p.cfg.OpenVPNCommand, "--config", confPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.mu.Unlock()
		p.StopTunnel(tunnelID)
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		t.mu.Unlock()
		p.StopTunnel(tunnelID)
		return nil, fmt.Errorf("启动 OpenVPN 进程失败: %w", err)
	}
	t.cmd = cmd
	t.mu.Unlock()

	// Monitor subprocess
	go func() {
		defer func() {
			t.mu.Lock()
			statusBefore := t.Status
			wasConnected := statusBefore == StatusConnected
			wasStopped := statusBefore == StatusStopped
			connectedAt := t.ConnectedAt
			message := t.Message
			if t.Status == StatusConnecting || t.Status == StatusConnected {
				t.Status = StatusFailed
				t.Message = "进程退出"
			}
			node := t.Node
			authPath := t.authPath
			confPath := t.confPath
			devIdx := t.DevIndex
			t.mu.Unlock()

			if p.nodePool != nil && node != nil {
				if wasConnected || !connectedAt.IsZero() {
					uptimeSec := int64(time.Since(connectedAt).Seconds())
					if p.nodePool.Reputation() != nil {
						p.nodePool.Reputation().RecordUptime(node.IP, node.ID, uptimeSec)
						if uptimeSec < 180 {
							p.nodePool.Reputation().RecordFail(node.IP, node.ID, true)
						}
					}
				} else if !wasStopped {
					if p.nodePool.Reputation() != nil {
						p.nodePool.Reputation().RecordFail(node.IP, node.ID, false)
					}
					reason := "握手未完成或连接被重置"
					if strings.Contains(message, "AUTH_FAILED") {
						reason = "身份认证失败被拒绝"
					} else if strings.Contains(message, "TLS Error") || strings.Contains(message, "Connection reset") {
						reason = "连接被对端重置或TLS协商失败"
					}
					if p.nodePool.Blacklist() != nil {
						p.nodePool.Blacklist().Mark(node, reason, 900*time.Second)
					}
				}
			}

			_ = cmd.Wait()

			// Clean up auth/config files
			if authPath != "" {
				_ = os.Remove(authPath)
			}
			if confPath != "" {
				_ = os.Remove(confPath)
			}

			teardownTunnelInterface(devName, devIdx)

			// Immediately recycle device index and remove dead tunnel so it doesn't leak virtual NICs
			p.mu.Lock()
			p.freeDevIndexLocked(devIdx)
			delete(p.tunnels, tunnelID)
			p.mu.Unlock()

			stats.LogInfo("TunnelPool", "隧道 [%s] (%s) 进程已终止，已回收虚拟网卡和设备槽位", tunnelID, devName)
		}()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			stats.LogInfo(fmt.Sprintf("VPN:%s", devName), "%s", line)

			if strings.Contains(line, "Initialization Sequence Completed") {
				setupTunnelInterface(devName, devIdx)

				t.mu.Lock()
				t.Status = StatusConnected
				t.ConnectedAt = time.Now()
				t.Message = "已连接并就绪"
				t.mu.Unlock()

				stats.LogInfo("TunnelPool", "隧道 [%s] (%s) 已成功连通并就绪路由！", tunnelID, devName)

				if p.nodePool != nil && p.nodePool.Reputation() != nil && t.Node != nil {
					p.nodePool.Reputation().RecordSuccess(t.Node.IP, t.Node.ID)
				}
				go p.probeUnlock(tunnelID)
			} else if strings.Contains(line, "AUTH_FAILED") {
				t.mu.Lock()
				t.Status = StatusFailed
				t.Message = "身份认证失败"
				t.mu.Unlock()
			} else if strings.Contains(line, "TLS Error") || strings.Contains(line, "Connection reset by peer") {
				t.mu.Lock()
				t.Message = line
				t.mu.Unlock()
			}
		}
	}()

	// Handshake watchdog: if connecting for > 25s without completing, terminate and recycle
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(25 * time.Second):
			t.mu.RLock()
			st := t.Status
			t.mu.RUnlock()
			if st == StatusConnecting {
				stats.LogWarn("TunnelPool", "隧道 [%s] (%s) 握手超时 (25s)，自动终止并回收资源", tunnelID, devName)
				t.mu.Lock()
				if t.cancelFunc != nil {
					t.cancelFunc()
				}
				t.mu.Unlock()
			}
		}
	}()

	return t, nil
}

func (p *Pool) probeUnlock(tunnelID string) {
	p.mu.RLock()
	t, ok := p.tunnels[tunnelID]
	p.mu.RUnlock()
	if !ok || t.Node == nil {
		return
	}

	if p.unlockDetector == nil {
		return
	}

	if cached := p.unlockDetector.GetUnlock(t.Node.IP); cached != nil {
		t.mu.Lock()
		t.Unlock = cached
		t.mu.Unlock()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res := p.unlockDetector.ProbeTunnel(ctx, t.DevName, t.Node.IP)
	t.mu.Lock()
	t.Unlock = res
	t.mu.Unlock()
}

func (p *Pool) StopTunnel(tunnelID string) error {
	p.mu.Lock()
	t, ok := p.tunnels[tunnelID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("隧道 %s 不存在", tunnelID)
	}
	delete(p.tunnels, tunnelID)
	devIdx := t.DevIndex
	p.mu.Unlock()

	t.mu.Lock()
	t.Status = StatusStopped
	t.Message = "已手动停止"

	if t.cancelFunc != nil {
		t.cancelFunc()
		t.cancelFunc = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
		t.cmd = nil
	}

	if t.authPath != "" {
		_ = os.Remove(t.authPath)
	}
	if t.confPath != "" {
		_ = os.Remove(t.confPath)
	}
	t.mu.Unlock()

	teardownTunnelInterface(t.DevName, devIdx)

	// Free dev index only after the process has completely terminated
	p.mu.Lock()
	p.freeDevIndexLocked(devIdx)
	p.mu.Unlock()

	stats.LogInfo("TunnelPool", "隧道 [%s] (%s) 已关闭释放", tunnelID, t.DevName)
	return nil
}

func setupTunnelInterface(devName string, devIndex int) {
	if devName == "" {
		return
	}
	tableID := 100 + devIndex

	// 1. Enable loose reverse path filtering on the tunnel device to allow responses
	_ = exec.Command("sysctl", "-w", fmt.Sprintf("net.ipv4.conf.%s.rp_filter=2", devName)).Run()
	_ = exec.Command("sysctl", "-w", "net.ipv4.conf.all.rp_filter=2").Run()

	// 2. Add policy routing rule and default route STRICTLY inside isolated tableID
	// NEVER touch the main routing table so eth0 default gateway and SSH port 22 are 100% untouched
	_ = exec.Command("ip", "route", "replace", "default", "dev", devName, "table", fmt.Sprintf("%d", tableID)).Run()
	_ = exec.Command("ip", "rule", "del", "oif", devName, "table", fmt.Sprintf("%d", tableID)).Run()
	_ = exec.Command("ip", "rule", "add", "oif", devName, "table", fmt.Sprintf("%d", tableID), "priority", "1000").Run()

	stats.LogInfo("TunnelPool", "已为接口 %s 配置独立隔离策略路由 (Table %d) 与 rp_filter", devName, tableID)
}

func teardownTunnelInterface(devName string, devIndex int) {
	if devName == "" {
		return
	}
	tableID := 100 + devIndex
	_ = exec.Command("ip", "rule", "del", "oif", devName, "table", fmt.Sprintf("%d", tableID)).Run()
	_ = exec.Command("ip", "route", "del", "default", "dev", devName, "table", fmt.Sprintf("%d", tableID)).Run()
}

func (p *Pool) ListTunnels() []*Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()

	res := make([]*Tunnel, 0, len(p.tunnels))
	for _, t := range p.tunnels {
		t.mu.RLock()
		st := t.Status
		t.mu.RUnlock()
		if st == StatusConnecting || st == StatusConnected {
			res = append(res, t.Snapshot())
		}
	}

	// Stably sort by virtual device index ascending (tun0, tun1, tun2...)
	sort.Slice(res, func(i, j int) bool {
		return res[i].DevIndex < res[j].DevIndex
	})
	return res
}

func (p *Pool) ReapStaleTunnels() {
	p.mu.Lock()
	p.reapDeadTunnelsLocked()
	p.mu.Unlock()
}

func (p *Pool) GetTunnel(id string) *Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tunnels[id]
}

func (p *Pool) GetHealthyTunnels(targetIDs []string) []*Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()

	idFilter := make(map[string]bool)
	for _, id := range targetIDs {
		idFilter[id] = true
	}

	// 1. Prioritize available (not circuit-broken) tunnels
	var available []*Tunnel
	var healthy []*Tunnel
	for _, t := range p.tunnels {
		if len(idFilter) == 0 || idFilter[t.ID] {
			if t.IsAvailable() {
				available = append(available, t)
			}
			if t.IsHealthy() {
				healthy = append(healthy, t)
			}
		}
	}

	if len(available) > 0 {
		return available
	}
	return healthy
}

func (p *Pool) CloseAll() {
	p.mu.RLock()
	ids := make([]string, 0, len(p.tunnels))
	for id := range p.tunnels {
		ids = append(ids, id)
	}
	p.mu.RUnlock()

	for _, id := range ids {
		_ = p.StopTunnel(id)
	}
}
