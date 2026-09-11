package config

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	mu sync.RWMutex

	// API & Mirror sources
	ApiURL        string
	MirrorURL     string
	MirrorMetaURL string

	// Web UI settings
	UIHost     string
	UIPort     int
	UIPath     string
	UIUsername string
	UIPassword string

	// Proxy gateway settings
	ProxyHost           string
	ProxyPort           int
	ProxyUser           string
	ProxyPass           string
	ProxyMaxConnections int

	// Intervals & Limits
	FetchInterval      time.Duration
	CheckInterval      time.Duration
	TargetValidNodes   int
	MaxScanRows        int
	InvalidBackoff     time.Duration
	DiscoveryCountries []string

	// 自动轮换策略
	AutoRotateMinutes int    // 0 = 禁用, > 0 轮换周期分钟
	AutoRotateIPType  string // all / residential / hosting

	// Telegram 机器人告警与远程交互
	TelegramBotToken string
	TelegramChatID   string

	// OpenVPN parameters
	OpenVPNCommand  string
	OpenVPNAuthUser string
	OpenVPNAuthPass string

	// Data directory
	DataDir string
}

type SettingsDTO struct {
	UIPort             int      `json:"ui_port"`
	UIPath             string   `json:"ui_path"`
	UIUsername         string   `json:"ui_username"`
	UIPassword         string   `json:"ui_password,omitempty"`
	ProxyPort          int      `json:"proxy_port"`
	ProxyUser          string   `json:"proxy_user"`
	ProxyPass          string   `json:"proxy_pass,omitempty"`
	AutoRotateMinutes  int      `json:"auto_rotate_minutes"`
	AutoRotateIPType   string   `json:"auto_rotate_ip_type"`
	DiscoveryCountries []string `json:"discovery_countries"`
	TelegramBotToken   string   `json:"telegram_bot_token"`
	TelegramChatID     string   `json:"telegram_chat_id"`
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return strings.TrimSpace(val)
}

func getEnvInt(key string, defaultVal, minVal, maxVal int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultVal
	}
	if minVal != 0 || maxVal != 0 {
		if val < minVal {
			return minVal
		}
		if val > maxVal {
			return maxVal
		}
	}
	return val
}

func randomSecretPath() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "aimili"
	}
	return hex.EncodeToString(b)
}

func LoadConfig() *Config {
	dataDir := getEnv("DATA_DIR", "data")
	_ = os.MkdirAll(dataDir, 0700)
	// #nosec G302 -- 0700 is restrictive for a directory that contains config and credential files.
	_ = os.Chmod(dataDir, 0700)

	uiPath := getEnv("UI_PATH", "")
	if uiPath == "" {
		uiPath = getEnv("ADMIN_SECRET_PATH", "")
	}
	if uiPath == "" {
		// Check if saved before
		secretFile := filepath.Join(dataDir, "admin_path.txt")
		// #nosec G304 -- path is rooted in the operator-controlled data directory.
		if data, err := os.ReadFile(secretFile); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			uiPath = strings.TrimSpace(string(data))
		} else {
			uiPath = randomSecretPath()
			_ = os.WriteFile(secretFile, []byte(uiPath), 0600)
			_ = os.Chmod(secretFile, 0600)
		}
	}
	uiPath = strings.Trim(uiPath, "/")

	countriesRaw := getEnv("DISCOVERY_COUNTRIES", "")
	var countries []string
	if countriesRaw != "" {
		for _, c := range strings.Split(countriesRaw, ",") {
			c = strings.ToUpper(strings.TrimSpace(c))
			if len(c) == 2 {
				countries = append(countries, c)
			}
		}
	}

	return &Config{
		ApiURL:        getEnv("VPNGATE_API_HTTPS_URL", "https://www.vpngate.net/api/iphone/"),
		MirrorURL:     getEnv("VPNGATE_MIRROR_HTTPS_URL", "https://baoweise-bot.github.io/aimili-vpngate/vpngate.csv"),
		MirrorMetaURL: getEnv("VPNGATE_MIRROR_META_URL", "https://baoweise-bot.github.io/aimili-vpngate/vpngate.meta.json"),

		UIHost:     getEnv("UI_HOST", "::"),
		UIPort:     getEnvInt("UI_PORT", 8787, 1, 65535),
		UIPath:     uiPath,
		UIUsername: getEnv("UI_USERNAME", "admin"),
		UIPassword: getEnv("UI_PASSWORD", "aimilivpn"),

		ProxyHost:           getEnv("LOCAL_PROXY_HOST", "127.0.0.1"),
		ProxyPort:           getEnvInt("LOCAL_PROXY_PORT", 7928, 1, 65535),
		ProxyUser:           getEnv("LOCAL_PROXY_USER", ""),
		ProxyPass:           getEnv("LOCAL_PROXY_PASS", ""),
		ProxyMaxConnections: getEnvInt("LOCAL_PROXY_MAX_CONNECTIONS", 512, 16, 4096),

		FetchInterval:      time.Duration(getEnvInt("FETCH_INTERVAL_SECONDS", 900, 60, 86400)) * time.Second,
		CheckInterval:      time.Duration(getEnvInt("CHECK_INTERVAL_SECONDS", 20, 5, 300)) * time.Second,
		TargetValidNodes:   getEnvInt("TARGET_VALID_NODES", 5, 1, 50),
		MaxScanRows:        getEnvInt("MAX_SCAN_ROWS", 100, 10, 500),
		InvalidBackoff:     time.Duration(getEnvInt("INVALID_BACKOFF_SECONDS", 1800, 60, 86400)) * time.Second,
		DiscoveryCountries: countries,

		AutoRotateMinutes: getEnvInt("AUTO_ROTATE_INTERVAL_MINUTES", 0, 0, 1440),
		AutoRotateIPType:  getEnv("AUTO_ROTATE_IP_TYPE", "all"),

		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),

		OpenVPNCommand:  getEnv("OPENVPN_CMD", "openvpn"),
		OpenVPNAuthUser: getEnv("OPENVPN_AUTH_USER", "vpn"),
		OpenVPNAuthPass: getEnv("OPENVPN_AUTH_PASS", "vpn"),

		DataDir: dataDir,
	}
}

func (c *Config) GetSettings() SettingsDTO {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return SettingsDTO{
		UIPort:             c.UIPort,
		UIPath:             c.UIPath,
		UIUsername:         c.UIUsername,
		ProxyPort:          c.ProxyPort,
		ProxyUser:          c.ProxyUser,
		AutoRotateMinutes:  c.AutoRotateMinutes,
		AutoRotateIPType:   c.AutoRotateIPType,
		DiscoveryCountries: c.DiscoveryCountries,
		TelegramBotToken:   c.TelegramBotToken,
		TelegramChatID:     c.TelegramChatID,
	}
}

func (c *Config) VerifyUICredentials(user, pass string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.UIUsername == "" && c.UIPassword == "" {
		return true
	}

	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.UIUsername)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.UIPassword)) == 1
	return userMatch && passMatch
}

func (c *Config) IsUIAuthEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.UIUsername != "" || c.UIPassword != ""
}

func (c *Config) UpdateSettings(dto SettingsDTO) error {
	for name, value := range map[string]string{
		"ui_path":            dto.UIPath,
		"ui_username":        dto.UIUsername,
		"ui_password":        dto.UIPassword,
		"proxy_user":         dto.ProxyUser,
		"proxy_pass":         dto.ProxyPass,
		"telegram_bot_token": dto.TelegramBotToken,
		"telegram_chat_id":   dto.TelegramChatID,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s 不能包含换行符", name)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if dto.UIPort > 0 && dto.UIPort < 65536 {
		c.UIPort = dto.UIPort
	}
	if dto.ProxyPort > 0 && dto.ProxyPort < 65536 {
		c.ProxyPort = dto.ProxyPort
	}
	if strings.TrimSpace(dto.UIPath) != "" {
		c.UIPath = strings.Trim(strings.TrimSpace(dto.UIPath), "/")
	}
	if strings.TrimSpace(dto.UIUsername) != "" {
		c.UIUsername = strings.TrimSpace(dto.UIUsername)
	}
	if strings.TrimSpace(dto.UIPassword) != "" {
		c.UIPassword = strings.TrimSpace(dto.UIPassword)
	}
	if dto.ProxyUser != "" {
		c.ProxyUser = strings.TrimSpace(dto.ProxyUser)
	}
	if dto.ProxyPass != "" {
		c.ProxyPass = strings.TrimSpace(dto.ProxyPass)
	}
	if dto.AutoRotateMinutes >= 0 {
		c.AutoRotateMinutes = dto.AutoRotateMinutes
	}
	if dto.AutoRotateIPType != "" {
		c.AutoRotateIPType = dto.AutoRotateIPType
	}
	if dto.DiscoveryCountries != nil {
		c.DiscoveryCountries = dto.DiscoveryCountries
	}
	c.TelegramBotToken = strings.TrimSpace(dto.TelegramBotToken)
	c.TelegramChatID = strings.TrimSpace(dto.TelegramChatID)
	if dto.ProxyPass != "" {
		c.ProxyPass = strings.TrimSpace(dto.ProxyPass)
	}

	// Persist to config.env
	targetEnvPaths := []string{
		"/opt/aimilivpn/config.env",
		filepath.Join(c.DataDir, "../config.env"),
		"config.env",
	}

	for _, p := range targetEnvPaths {
		dir := filepath.Dir(p)
		if _, err := os.Stat(dir); err == nil {
			content := fmt.Sprintf(`# AimiliVPN 运行环境变量配置
DATA_DIR=%s
UI_HOST=%s
UI_PORT=%d
UI_PATH=%s
UI_USERNAME=%s
UI_PASSWORD=%s
LOCAL_PROXY_HOST=%s
LOCAL_PROXY_PORT=%d
LOCAL_PROXY_USER=%s
LOCAL_PROXY_PASS=%s
LOCAL_PROXY_MAX_CONNECTIONS=%d
CHECK_INTERVAL_SECONDS=%d
FETCH_INTERVAL_SECONDS=%d
TARGET_VALID_NODES=%d
AUTO_ROTATE_INTERVAL_MINUTES=%d
AUTO_ROTATE_IP_TYPE=%s
DISCOVERY_COUNTRIES=%s
TELEGRAM_BOT_TOKEN=%s
TELEGRAM_CHAT_ID=%s
`,
				c.DataDir,
				c.UIHost,
				c.UIPort,
				c.UIPath,
				c.UIUsername,
				c.UIPassword,
				c.ProxyHost,
				c.ProxyPort,
				c.ProxyUser,
				c.ProxyPass,
				c.ProxyMaxConnections,
				int(c.CheckInterval.Seconds()),
				int(c.FetchInterval.Seconds()),
				c.TargetValidNodes,
				c.AutoRotateMinutes,
				c.AutoRotateIPType,
				strings.Join(c.DiscoveryCountries, ","),
				c.TelegramBotToken,
				c.TelegramChatID,
			)
			if err := os.WriteFile(p, []byte(content), 0600); err != nil {
				return fmt.Errorf("写入配置文件 %s 失败: %w", p, err)
			}
			_ = os.Chmod(p, 0600)
			return nil
		}
	}

	return fmt.Errorf("未找到可写的配置文件路径")
}
