package vpn

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

type ProcessEvent string

const (
	EventConnected ProcessEvent = "CONNECTED"
	EventAuthFail  ProcessEvent = "AUTH_FAIL"
	EventError     ProcessEvent = "ERROR"
	EventExited    ProcessEvent = "EXITED"
)

type ProcessListener func(event ProcessEvent, message string)

type OpenVPNRunner struct {
	cfg      *config.Config
	cmd      *exec.Cmd
	mu       sync.Mutex
	authFile string
	confFile string
	running  bool
}

func NewOpenVPNRunner(cfg *config.Config) *OpenVPNRunner {
	return &OpenVPNRunner{
		cfg: cfg,
	}
}

func (r *OpenVPNRunner) PrepareFiles(node *nodes.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := r.cfg.DataDir
	_ = os.MkdirAll(dir, 0700)

	// 1. Write auth file
	r.authFile = filepath.Join(dir, "openvpn_auth.txt")
	authContent := fmt.Sprintf("%s\n%s\n", r.cfg.OpenVPNAuthUser, r.cfg.OpenVPNAuthPass)
	if err := os.WriteFile(r.authFile, []byte(authContent), 0600); err != nil {
		return fmt.Errorf("write openvpn auth file failed: %w", err)
	}

	// 2. Prepare tailored config
	r.confFile = filepath.Join(dir, "active.ovpn")
	lines := strings.Split(node.ConfigData, "\n")
	var modified []string

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		// Strip lines we want to control
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

	// Append AimiliVPN operational directives
	// 严禁使用 redirect-gateway! 必须使用 route-nopull 保证 VPS 原本的默认网关与 SSH 22 端口不受任何影响！
	// 仅代理网关流量通过 SO_BINDTODEVICE 绑定至 tun0 发送。
	modified = append(modified,
		"auth-user-pass "+r.authFile,
		"dev tun0",
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

	fullConfig := strings.Join(modified, "\n")
	if err := os.WriteFile(r.confFile, []byte(fullConfig), 0600); err != nil {
		return fmt.Errorf("write active.ovpn failed: %w", err)
	}

	return nil
}

func (r *OpenVPNRunner) Start(ctx context.Context, listener ProcessListener) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("openvpn runner already active")
	}

	cmd := exec.CommandContext(ctx, r.cfg.OpenVPNCommand, "--config", r.confFile)
	// Give process its own process group so we can kill all subprocesses cleanly
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	cmd.Stderr = cmd.Stdout // Merge stderr to stdout

	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("start openvpn binary '%s' failed: %w", r.cfg.OpenVPNCommand, err)
	}

	r.cmd = cmd
	r.running = true
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
			r.Cleanup()
			listener(EventExited, "OpenVPN 进程已退出")
		}()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			stats.LogInfo("OpenVPN", "%s", line)

			if strings.Contains(line, "Initialization Sequence Completed") {
				listener(EventConnected, "VPN 隧道建立完成 (Initialization Sequence Completed)")
			} else if strings.Contains(line, "AUTH_FAILED") {
				listener(EventAuthFail, "VPN 节点身份认证失败")
			} else if strings.Contains(line, "TLS Error") || strings.Contains(line, "Connection reset by peer") {
				listener(EventError, line)
			}
		}

		_ = cmd.Wait()
	}()

	return nil
}

func (r *OpenVPNRunner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil && r.cmd.Process != nil {
		// Send SIGTERM to process group
		_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
		_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL)
		r.cmd = nil
	}
	r.running = false
}

func (r *OpenVPNRunner) Cleanup() {
	if r.authFile != "" {
		_ = os.Remove(r.authFile)
	}
	if r.confFile != "" {
		_ = os.Remove(r.confFile)
	}
}
