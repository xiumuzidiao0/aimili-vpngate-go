package nodes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aimili-vpngate-go/pkg/stats"
)

type Fetcher struct {
	apiURL    string
	mirrorURL string
	snapshot  *SnapshotManager
	client    *http.Client
}

func NewFetcher(apiURL, mirrorURL string, snapshot *SnapshotManager) *Fetcher {
	return &Fetcher{
		apiURL:    apiURL,
		mirrorURL: mirrorURL,
		snapshot:  snapshot,
		client: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

func (f *Fetcher) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AimiliVPN-Go/2.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status: %d", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		return nil, fmt.Errorf("received HTML block page instead of CSV (possible ISP interception)")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSnapshotBytes))
	if err != nil {
		return nil, err
	}

	// Validate content is actually VPNGate CSV and not an ISP intercept error
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("<")) || bytes.HasPrefix(trimmed, []byte("<!DOCTYPE")) {
		return nil, fmt.Errorf("response contains HTML tags, not a valid VPNGate CSV")
	}

	if !bytes.Contains(body, []byte("HostName")) && !bytes.Contains(body, []byte("vpn_servers")) {
		return nil, fmt.Errorf("response does not contain required VPNGate CSV headers")
	}

	return body, nil
}

type FetchResult struct {
	Data   []byte
	Source string
}

func (f *Fetcher) FetchNodes(ctx context.Context) (*FetchResult, error) {
	sources := []struct {
		name string
		url  string
	}{
		{"jsDelivr 全球加速 CDN", "https://cdn.jsdelivr.net/gh/baoweise-bot/aimili-vpngate@main/mirror/vpngate.csv"},
		{"Fastly 全球加速 CDN", "https://fastly.jsdelivr.net/gh/baoweise-bot/aimili-vpngate@main/mirror/vpngate.csv"},
		{"GitHub Pages 镜像源", f.mirrorURL},
		{"GitHub Raw 直链镜像", "https://raw.githubusercontent.com/baoweise-bot/aimili-vpngate/main/mirror/vpngate.csv"},
		{"用户 GitHub 镜像源", "https://raw.githubusercontent.com/xiumuzidiao0/aimili-vpngate-go/main/mirror/vpngate.csv"},
		{"VPNGate 官方 HTTPS API", f.apiURL},
		{"VPNGate 官方 HTTP API", "http://www.vpngate.net/api/iphone/"},
	}

	for _, src := range sources {
		if src.url == "" {
			continue
		}
		stats.LogInfo("Nodes", "正在尝试拉取节点列表 [%s]: %s", src.name, src.url)
		fetchCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		data, err := f.fetchURL(fetchCtx, src.url)
		cancel()

		if err == nil && len(data) > 0 {
			stats.LogInfo("Nodes", "成功从 [%s] 获取到 %d 字节数据", src.name, len(data))
			return &FetchResult{
				Data:   data,
				Source: src.name,
			}, nil
		}
		stats.LogWarn("Nodes", "从 [%s] 获取失败: %v，准备切换下一数据源", src.name, err)
	}

	// Fallback to local snapshot
	stats.LogInfo("Nodes", "网络源均不可用，尝试从本地快照恢复节点列表...")
	data, meta, err := f.snapshot.Load()
	if err == nil && len(data) > 0 {
		sourceName := "本地快照"
		if meta != nil && meta.Source != "" {
			sourceName = fmt.Sprintf("本地快照 (%s)", meta.Source)
		}
		stats.LogInfo("Nodes", "成功加载本地快照 (%d 字节)", len(data))
		return &FetchResult{
			Data:   data,
			Source: sourceName,
		}, nil
	}

	return nil, fmt.Errorf("所有在线镜像源及本地快照均不可用")
}
