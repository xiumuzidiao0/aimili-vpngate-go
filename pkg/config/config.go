package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
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
	FetchInterval       time.Duration
	CheckInterval       time.Duration
	TargetValidNodes    int
	MaxScanRows         int
	InvalidBackoff      time.Duration
	DiscoveryCountries  []string

	// OpenVPN parameters
	OpenVPNCommand      string
	OpenVPNAuthUser     string
	OpenVPNAuthPass     string

	// Data directory
	DataDir string
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
	_ = os.MkdirAll(dataDir, 0755)

	uiPath := getEnv("UI_PATH", "")
	if uiPath == "" {
		uiPath = getEnv("ADMIN_SECRET_PATH", "")
	}
	if uiPath == "" {
		// Check if saved before
		secretFile := filepath.Join(dataDir, "admin_path.txt")
		if data, err := os.ReadFile(secretFile); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			uiPath = strings.TrimSpace(string(data))
		} else {
			uiPath = randomSecretPath()
			_ = os.WriteFile(secretFile, []byte(uiPath), 0600)
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

		OpenVPNCommand:  getEnv("OPENVPN_CMD", "openvpn"),
		OpenVPNAuthUser: getEnv("OPENVPN_AUTH_USER", "vpn"),
		OpenVPNAuthPass: getEnv("OPENVPN_AUTH_PASS", "vpn"),

		DataDir: dataDir,
	}
}
