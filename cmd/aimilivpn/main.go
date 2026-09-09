package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/proxy"
	"aimili-vpngate-go/pkg/server"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/vpn"
)

const version = "2.0.0-go"

func main() {
	showVersion := flag.Bool("version", false, "显示程序版本号")
	flag.Parse()

	if *showVersion {
		fmt.Printf("AimiliVPN Go Gateway v%s\n", version)
		os.Exit(0)
	}

	cfg := config.LoadConfig()

	// Initialize log ring buffer & traffic meter
	_ = stats.InitRingLog(1000)
	stats.StartTrafficTicker()

	stats.LogInfo("Main", "=== AimiliVPN 代理网关 (Go 高性能重构版 v%s) 启动中 ===", version)

	// Preflight checks
	if err := vpn.CheckTUNDevice(); err != nil {
		stats.LogWarn("Main", "警告: %v", err)
	}
	vpn.KillStrayOpenVPN()

	// Initialize components
	nodePool := nodes.NewNodePool(cfg)
	vpnMgr := vpn.NewManager(cfg, nodePool)
	gateway := proxy.NewGateway(cfg)
	webServer := server.NewServer(cfg, nodePool, vpnMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start Proxy Gateway
	go func() {
		if err := gateway.Start(ctx); err != nil {
			stats.LogError("Gateway", "代理网关异常退出: %v", err)
		}
	}()

	// 2. Start Web Server
	go func() {
		if err := webServer.Start(ctx); err != nil {
			stats.LogError("Server", "Web 服务异常退出: %v", err)
		}
	}()

	// 3. Start VPN Health Checker and Auto Rotator
	vpnMgr.StartHealthChecker(ctx)
	vpnMgr.StartAutoRotator(ctx)

	// 4. Initial fetch of nodes and auto-connect
	go func() {
		time.Sleep(500 * time.Millisecond)
		stats.LogInfo("Main", "正在初始化节点池并拉取候选节点...")
		if err := nodePool.Refresh(ctx); err != nil {
			stats.LogWarn("Main", "初次拉取节点失败: %v，等待定时任务重试", err)
		} else {
			// Auto connect to best node if available
			best := nodePool.SelectBest()
			if best != nil {
				stats.LogInfo("Main", "检测到最优候选节点 [%s] (%s)，准备自动建立 VPN 连接...", best.ID, best.CountryShort)
				_ = vpnMgr.Connect(best)
			}
		}

		// Periodic node list refresh
		ticker := time.NewTicker(cfg.FetchInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats.LogInfo("Main", "正在执行周期性节点列表增量刷新...")
				_ = nodePool.Refresh(ctx)
			}
		}
	}()

	// Print startup summary banner
	adminURL := fmt.Sprintf("http://%s:%d/%s", cfg.UIHost, cfg.UIPort, strings.Trim(cfg.UIPath, "/"))
	if cfg.UIHost == "::" || cfg.UIHost == "0.0.0.0" {
		adminURL = fmt.Sprintf("http://<VPS-IP>:%d/%s", cfg.UIPort, strings.Trim(cfg.UIPath, "/"))
	}
	stats.LogInfo("Main", "-------------------------------------------------------------")
	stats.LogInfo("Main", "Web 管理控制台 : %s", adminURL)
	stats.LogInfo("Main", "管理账号 / 密码 : %s / %s", cfg.UIUsername, cfg.UIPassword)
	stats.LogInfo("Main", "本地代理网关   : %s:%d (支持 SOCKS5 / HTTP / HTTPS CONNECT)", cfg.ProxyHost, cfg.ProxyPort)
	stats.LogInfo("Main", "Prometheus 指标: http://<VPS-IP>:%d/metrics", cfg.UIPort)
	stats.LogInfo("Main", "-------------------------------------------------------------")

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-sigCh

	stats.LogInfo("Main", "收到系统退出信号 (%v)，正在安全退出...", sig)
	cancel()

	// Graceful cleanup
	vpnMgr.Disconnect("程序收到终止信号退出")
	gateway.Close()

	// Wait brief moment for processes and sockets to release
	time.Sleep(500 * time.Millisecond)
	stats.LogInfo("Main", "AimiliVPN 服务已成功安全停止。")
}
