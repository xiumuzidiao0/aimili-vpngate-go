package nodes

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
			Timeout: 20 * time.Second,
		},
	}
}

func (f *Fetcher) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AimiliVPN-Go/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSnapshotBytes))
	if err != nil {
		return nil, err
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
		{"VPNGate 官方 API", f.apiURL},
		{"GitHub 镜像源", f.mirrorURL},
	}

	for _, src := range sources {
		if src.url == "" {
			continue
		}
		stats.LogInfo("Nodes", "正在尝试拉取节点列表 [%s]: %s", src.name, src.url)
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
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
	stats.LogInfo("Nodes", "尝试从本地快照恢复节点列表...")
	data, meta, err := f.snapshot.Load()
	if err == nil && len(data) > 0 {
		sourceName := "本地快照"
		if meta != nil && meta.Source != "" {
			sourceName = fmt.Sprintf("本地快照 (%s, %s)", meta.Source, meta.CachedAt.Format("2006-01-02 15:04:05"))
		}
		stats.LogInfo("Nodes", "成功加载本地快照 (%d 字节)", len(data))
		return &FetchResult{
			Data:   data,
			Source: sourceName,
		}, nil
	}

	return nil, fmt.Errorf("所有节点源均拉取失败，且无可用本地快照")
}
