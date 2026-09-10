package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/stats"
)

type CommandHandler func(cmd, args string) string

type TelegramNotifier struct {
	cfg    *config.Config
	client *http.Client
	mu     sync.RWMutex
	lastID int64
}

func NewTelegramNotifier(cfg *config.Config) *TelegramNotifier {
	return &TelegramNotifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (t *TelegramNotifier) IsConfigured() bool {
	settings := t.cfg.GetSettings()
	return settings.TelegramBotToken != "" && settings.TelegramChatID != ""
}

func (t *TelegramNotifier) SendMessage(text string) error {
	settings := t.cfg.GetSettings()
	token := strings.TrimSpace(settings.TelegramBotToken)
	chatID := strings.TrimSpace(settings.TelegramChatID)
	if token == "" || chatID == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		stats.LogWarn("Telegram", "发送告警消息失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		stats.LogWarn("Telegram", "发送告警被拒绝: %v", err)
		return err
	}

	return nil
}

func (t *TelegramNotifier) NotifyStartup(ver string, port int, path string) {
	msg := fmt.Sprintf("🚀 <b>AimiliVPN 代理网关已就绪</b>\n\n"+
		"<b>版本:</b> v%s\n"+
		"<b>本地代理端口:</b> <code>%d</code> (HTTP/SOCKS5)\n"+
		"<b>安全管理入口:</b> <code>/%s</code>\n"+
		"<b>状态:</b> 守护服务已正常运行并在网监听",
		ver, port, path)
	_ = t.SendMessage(msg)
}

func (t *TelegramNotifier) NotifyFailover(oldID, newID, country string) {
	msg := fmt.Sprintf("⚠️ <b>节点故障自动转移通知</b>\n\n"+
		"<b>原节点:</b> <code>%s</code> (异常中断)\n"+
		"<b>新节点:</b> <code>%s</code> (%s)\n"+
		"<b>状态:</b> 已平滑自动切换至新候选节点",
		oldID, newID, country)
	_ = t.SendMessage(msg)
}

func (t *TelegramNotifier) NotifyPoolAlert(groupName string, msgDetail string) {
	msg := fmt.Sprintf("🚨 <b>自适应隧道组告警</b>\n\n"+
		"<b>隧道组:</b> %s\n"+
		"<b>详情:</b> %s\n"+
		"<b>建议:</b> 请检查 VPS 外网连通性或在控制台刷新节点源",
		groupName, msgDetail)
	_ = t.SendMessage(msg)
}

type tgUpdateResponse struct {
	Ok     bool `json:"ok"`
	Result []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"result"`
}

func (t *TelegramNotifier) StartPolling(ctx context.Context, handler CommandHandler) {
	go func() {
		// Wait brief moment after startup
		time.Sleep(3 * time.Second)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			settings := t.cfg.GetSettings()
			token := strings.TrimSpace(settings.TelegramBotToken)
			configuredChatID := strings.TrimSpace(settings.TelegramChatID)
			if token == "" {
				time.Sleep(10 * time.Second)
				continue
			}

			reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=15", token, t.lastID+1)
			req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			resp, err := t.client.Do(req)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			var updates tgUpdateResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&updates)
			_ = resp.Body.Close()
			if decodeErr != nil || !updates.Ok {
				time.Sleep(5 * time.Second)
				continue
			}

			for _, up := range updates.Result {
				if up.UpdateID >= t.lastID {
					t.lastID = up.UpdateID
				}
				if up.Message == nil || up.Message.Text == "" {
					continue
				}

				// Only allow configured chat ID to execute commands for safety
				chatIDStr := strconv.FormatInt(up.Message.Chat.ID, 10)
				if configuredChatID != "" && chatIDStr != configuredChatID {
					continue
				}

				text := strings.TrimSpace(up.Message.Text)
				if !strings.HasPrefix(text, "/") {
					continue
				}

				parts := strings.Fields(text)
				cmd := strings.ToLower(parts[0])
				// Strip @BotUsername if present, e.g. /status@MyBot
				if atIdx := strings.Index(cmd, "@"); atIdx != -1 {
					cmd = cmd[:atIdx]
				}

				args := ""
				if len(parts) > 1 {
					args = strings.Join(parts[1:], " ")
				}

				reply := ""
				if handler != nil {
					reply = handler(cmd, args)
				}

				if reply != "" {
					_ = t.SendMessage(reply)
				}
			}
		}
	}()
}
