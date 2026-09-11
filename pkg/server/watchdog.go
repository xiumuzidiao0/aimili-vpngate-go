package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/stats"
)

// SingBoxWatchdog periodically inspects the sing-box core service and auto-heals unexpected terminations.
type SingBoxWatchdog struct {
	server           *Server
	interval         time.Duration
	consecutiveFails int
	cooldownUntil    time.Time
	mu               sync.Mutex
}

func NewSingBoxWatchdog(s *Server, interval time.Duration) *SingBoxWatchdog {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &SingBoxWatchdog{
		server:   s,
		interval: interval,
	}
}

// Start launches the background watchdog loop.
func (w *SingBoxWatchdog) Start(ctx context.Context) {
	go func() {
		// Wait 10 seconds after server boot before first check
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}

		w.check(ctx)

		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.check(ctx)
			}
		}
	}()
}

func (w *SingBoxWatchdog) check(ctx context.Context) {
	if w.server == nil || w.server.singboxClient == nil {
		return
	}

	if !w.server.singboxClient.IsInstalled() {
		return
	}

	w.mu.Lock()
	if !w.cooldownUntil.IsZero() && time.Now().Before(w.cooldownUntil) {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	status, err := w.server.singboxClient.GetStatus(checkCtx)
	if err != nil || status == nil || !status.Installed {
		return
	}

	// Only heal if nodes are configured and the core process unexpectedly stopped
	if status.NodeCount > 0 && !status.Core.Running {
		stats.LogWarn("Watchdog", "⚠️ 检测到 sing-box 入站核心异常停止运行 (已配置 %d 个抗封锁节点)，触发自动自愈拉起...", status.NodeCount)

		healCtx, healCancel := context.WithTimeout(ctx, 15*time.Second)
		defer healCancel()

		err := w.server.singboxClient.RestartService(healCtx)
		w.mu.Lock()
		defer w.mu.Unlock()

		if err != nil {
			w.consecutiveFails++
			stats.LogError("Watchdog", "sing-box 自动拉起失败 (%d/3): %v", w.consecutiveFails, err)
			if w.consecutiveFails >= 3 {
				w.cooldownUntil = time.Now().Add(2 * time.Minute)
				w.consecutiveFails = 0
				stats.LogWarn("Watchdog", "sing-box 连续3次自动拉起未恢复，进入 2 分钟冷却保护期，暂停死循环重启")
			}
		} else {
			w.consecutiveFails = 0
			w.cooldownUntil = time.Time{}
			stats.LogInfo("Watchdog", "✅ sing-box 入站核心已被后台守护程序成功自愈拉起！")

			if w.server.notifier != nil && w.server.notifier.IsConfigured() {
				msg := fmt.Sprintf("⚠️ <b>AimiliVPN 警报与自愈</b>\n\n检测到 <b>sing-box</b> 入站服务异常中断，后台自愈守护程序已自动执行安全重启并恢复就绪！\n时间: %s", time.Now().Format("2006-01-02 15:04:05"))
				_ = w.server.notifier.SendMessage(msg)
			}
		}
	} else if status.Core.Running {
		// Healthy running state, reset fail counter
		w.mu.Lock()
		if w.consecutiveFails > 0 {
			w.consecutiveFails = 0
		}
		w.mu.Unlock()
	}
}
