package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
)

type Pool struct {
	cfg        *config.Config
	mu         sync.RWMutex
	tunnels    map[string]*Tunnel
	usedDevs   map[int]bool
	nodePool   *nodes.NodePool
	nextIDSeq  int
}

func NewPool(cfg *config.Config, np *nodes.NodePool) *Pool {
	return &Pool{
		cfg:      cfg,
		tunnels:  make(map[string]*Tunnel),
		usedDevs: make(map[int]bool),
		nodePool: np,
	}
}

func (p *Pool) allocDevIndexLocked() int {
	for i := 0; i < 64; i++ {
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

func (p *Pool) StartTunnel(node *nodes.Node) (*Tunnel, error) {
	p.mu.Lock()
	// Check if already connecting or connected to this node
	for _, t := range p.tunnels {
		if t.Node != nil && t.Node.ID == node.ID && (t.Status == StatusConnected || t.Status == StatusConnecting) {
			p.mu.Unlock()
			return t, fmt.Errorf("节点 %s 已经在运行中 (设备: %s)", node.ID, t.DevName)
		}
	}

	devIdx := p.allocDevIndexLocked()
	if devIdx < 0 {
		p.mu.Unlock()
		return nil, fmt.Errorf("并发隧道已达上限 (最多64个)")
	}

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
		"connect-retry 1 3",
		"connect-timeout 10",
		"ping 5",
		"ping-restart 15",
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
			if t.Status == StatusConnecting || t.Status == StatusConnected {
				t.Status = StatusFailed
				t.Message = "进程退出"
			}
			t.mu.Unlock()
			_ = cmd.Wait()
		}()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			stats.LogInfo(fmt.Sprintf("VPN:%s", devName), "%s", line)

			t.mu.Lock()
			if strings.Contains(line, "Initialization Sequence Completed") {
				t.Status = StatusConnected
				t.ConnectedAt = time.Now()
				t.Message = "已连接并就绪"
				stats.LogInfo("TunnelPool", "隧道 [%s] (%s) 已成功连通！", tunnelID, devName)
			} else if strings.Contains(line, "AUTH_FAILED") {
				t.Status = StatusFailed
				t.Message = "身份认证失败"
			} else if strings.Contains(line, "TLS Error") || strings.Contains(line, "Connection reset by peer") {
				t.Message = line
			}
			t.mu.Unlock()
		}
	}()

	return t, nil
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

	// Free dev index only after the process has completely terminated
	p.mu.Lock()
	p.freeDevIndexLocked(devIdx)
	p.mu.Unlock()

	stats.LogInfo("TunnelPool", "隧道 [%s] (%s) 已关闭释放", tunnelID, t.DevName)
	return nil
}

func (p *Pool) ListTunnels() []*Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()

	res := make([]*Tunnel, 0, len(p.tunnels))
	for _, t := range p.tunnels {
		res = append(res, t.Snapshot())
	}
	return res
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

	var res []*Tunnel
	for _, t := range p.tunnels {
		if (len(idFilter) == 0 || idFilter[t.ID]) && t.IsHealthy() {
			res = append(res, t)
		}
	}
	return res
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
