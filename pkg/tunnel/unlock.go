package tunnel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/stats"
)

type ServiceUnlockStatus string

const (
	StatusUnlocked ServiceUnlockStatus = "unlocked" // 解锁可用
	StatusBlocked  ServiceUnlockStatus = "blocked"  // 风控或地区封锁
	StatusUnknown  ServiceUnlockStatus = "unknown"  // 探测超时或未知
)

type UnlockResult struct {
	IP            string              `json:"ip"`
	OpenAI        ServiceUnlockStatus `json:"openai"`         // ChatGPT / OpenAI
	Claude        ServiceUnlockStatus `json:"claude"`         // Claude / Anthropic
	Google        ServiceUnlockStatus `json:"google"`         // Google Search / 204
	Netflix       ServiceUnlockStatus `json:"netflix"`        // Netflix
	NetflixRegion string              `json:"netflix_region"` // 地区代码，如 "JP", "US"
	CheckedAt     time.Time           `json:"checked_at"`
}

type UnlockDetector struct {
	cachePath string
	mu        sync.RWMutex
	cache     map[string]*UnlockResult
}

func NewUnlockDetector(dataDir string) *UnlockDetector {
	d := &UnlockDetector{
		cachePath: filepath.Join(dataDir, "unlock_cache.json"),
		cache:     make(map[string]*UnlockResult),
	}
	d.load()
	return d
}

func (d *UnlockDetector) load() {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := os.ReadFile(d.cachePath)
	if err != nil {
		return
	}

	var raw map[string]*UnlockResult
	if err := json.Unmarshal(data, &raw); err == nil {
		now := time.Now()
		cleaned := make(map[string]*UnlockResult)
		for ip, res := range raw {
			// 12-hour TTL
			if res != nil && now.Sub(res.CheckedAt) < 12*time.Hour {
				cleaned[ip] = res
			}
		}
		d.cache = cleaned
	}
}

func (d *UnlockDetector) saveLocked() {
	data, err := json.MarshalIndent(d.cache, "", "  ")
	if err != nil {
		return
	}
	tmp := d.cachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err == nil {
		_ = os.Rename(tmp, d.cachePath)
	}
}

func (d *UnlockDetector) GetUnlock(ip string) *UnlockResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if res, ok := d.cache[ip]; ok && time.Since(res.CheckedAt) < 12*time.Hour {
		cp := *res
		return &cp
	}
	return nil
}

func (d *UnlockDetector) GetAllCached() map[string]*UnlockResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	res := make(map[string]*UnlockResult, len(d.cache))
	for ip, r := range d.cache {
		cp := *r
		res[ip] = &cp
	}
	return res
}

func (d *UnlockDetector) ProbeTunnel(ctx context.Context, devName string, ip string) *UnlockResult {
	result := &UnlockResult{
		IP:        ip,
		OpenAI:    StatusUnknown,
		Claude:    StatusUnknown,
		Google:    StatusUnknown,
		Netflix:   StatusUnknown,
		CheckedAt: time.Now(),
	}

	client := newTunnelHTTPClient(devName, 6*time.Second)

	var wg sync.WaitGroup
	wg.Add(4)

	// 1. OpenAI / ChatGPT
	go func() {
		defer wg.Done()
		req, err := http.NewRequestWithContext(ctx, "GET", "https://ios.chat.openai.com/public-api/mobile/server_status/v1", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 400 || resp.StatusCode == 401 {
					body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
					if !strings.Contains(string(body), "blocked") && !strings.Contains(string(body), "Cloudflare") {
						result.OpenAI = StatusUnlocked
						return
					}
				}
				if resp.StatusCode == 403 {
					result.OpenAI = StatusBlocked
					return
				}
			}
		}

		// Fallback check to cdn-cgi trace
		reqTrace, errTrace := http.NewRequestWithContext(ctx, "GET", "https://chatgpt.com/cdn-cgi/trace", nil)
		if errTrace == nil {
			resp, err := client.Do(reqTrace)
			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				if resp.StatusCode == 200 && strings.Contains(string(body), "loc=") {
					result.OpenAI = StatusUnlocked
					return
				}
				if resp.StatusCode == 403 {
					result.OpenAI = StatusBlocked
					return
				}
			}
		}
	}()

	// 2. Claude / Anthropic
	go func() {
		defer wg.Done()
		req, err := http.NewRequestWithContext(ctx, "GET", "https://claude.ai/api/auth/session", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 401 {
					result.Claude = StatusUnlocked
				} else if resp.StatusCode == 403 {
					result.Claude = StatusBlocked
				}
			}
		}
	}()

	// 3. Google / YouTube (Generate 204)
	go func() {
		defer wg.Done()
		req, err := http.NewRequestWithContext(ctx, "GET", "https://www.google.com/generate_204", nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 204 {
					result.Google = StatusUnlocked
				} else if resp.StatusCode == 429 {
					result.Google = StatusBlocked
				}
			}
		}
	}()

	// 4. Netflix
	go func() {
		defer wg.Done()
		req, err := http.NewRequestWithContext(ctx, "GET", "https://www.netflix.com/title/81280792", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					result.Netflix = StatusUnlocked
				} else if resp.StatusCode == 403 {
					result.Netflix = StatusBlocked
				}
			}
		}
	}()

	wg.Wait()

	// Update cache
	d.mu.Lock()
	d.cache[ip] = result
	d.saveLocked()
	d.mu.Unlock()

	stats.LogInfo("UnlockDetector", "[%s:%s] 解锁检测结果: ChatGPT=%s, Claude=%s, Google=%s, Netflix=%s",
		devName, ip, result.OpenAI, result.Claude, result.Google, result.Netflix)

	return result
}
