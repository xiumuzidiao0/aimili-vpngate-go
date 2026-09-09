package nodes

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/stats"
)

type NodePool struct {
	cfg       *config.Config
	fetcher   *Fetcher
	snapshot  *SnapshotManager
	blacklist *BlacklistManager

	mu          sync.RWMutex
	candidates  []*Node
	lastUpdated time.Time
	lastSource  string
	lastStatus  string
}

func NewNodePool(cfg *config.Config) *NodePool {
	sm := NewSnapshotManager(cfg.DataDir)
	fetcher := NewFetcher(cfg.ApiURL, cfg.MirrorURL, sm)
	bm := NewBlacklistManager(cfg.DataDir)

	return &NodePool{
		cfg:        cfg,
		fetcher:    fetcher,
		snapshot:   sm,
		blacklist:  bm,
		lastStatus: "初始化中",
	}
}

func (np *NodePool) Refresh(ctx context.Context) error {
	np.mu.Lock()
	np.lastStatus = "正在拉取节点列表"
	np.mu.Unlock()

	result, err := np.fetcher.FetchNodes(ctx)
	if err != nil {
		np.mu.Lock()
		np.lastStatus = fmt.Sprintf("拉取失败: %v", err)
		np.mu.Unlock()
		return err
	}

	nodes, err := ParseVPNGateCSV(result.Data, np.cfg.MaxScanRows)
	if err != nil {
		np.mu.Lock()
		np.lastStatus = fmt.Sprintf("解析失败: %v", err)
		np.mu.Unlock()
		return err
	}

	// Cache successful snapshot
	_ = np.snapshot.Save(result.Data, result.Source, len(nodes))

	// Filter by discovery countries if configured
	var filtered []*Node
	allowedCountries := make(map[string]bool)
	for _, c := range np.cfg.DiscoveryCountries {
		allowedCountries[c] = true
	}

	for _, n := range nodes {
		if np.blacklist.IsBlacklisted(n.ID) {
			continue
		}
		if len(allowedCountries) > 0 && !allowedCountries[n.CountryShort] {
			continue
		}
		filtered = append(filtered, n)
	}

	// Default sort by Score desc, Ping asc
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Score != filtered[j].Score {
			return filtered[i].Score > filtered[j].Score
		}
		return filtered[i].Ping < filtered[j].Ping
	})

	np.mu.Lock()
	np.candidates = filtered
	np.lastUpdated = time.Now()
	np.lastSource = result.Source
	np.lastStatus = fmt.Sprintf("就绪 (可用节点 %d/%d)", len(filtered), len(nodes))
	np.mu.Unlock()

	stats.LogInfo("Nodes", "节点池刷新完成，当前优质候选节点: %d 个 (来自: %s)", len(filtered), result.Source)

	// Async ping probe for top nodes in background
	go np.probeTopNodes(filtered, 15)

	return nil
}

func (np *NodePool) probeTopNodes(nodeList []*Node, limit int) {
	if limit > len(nodeList) {
		limit = len(nodeList)
	}
	top := nodeList[:limit]

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Limit concurrent ping dials

	for _, n := range top {
		wg.Add(1)
		go func(target *Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			addr := fmt.Sprintf("%s:%d", target.IP, target.Port)
			conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
			if err == nil {
				_ = conn.Close()
				latency := int(time.Since(start).Milliseconds())
				np.mu.Lock()
				target.LatencyMs = latency
				target.LastChecked = time.Now()
				np.mu.Unlock()
			} else {
				np.mu.Lock()
				target.LatencyMs = -1
				target.LastChecked = time.Now()
				np.mu.Unlock()
			}
		}(n)
	}
	wg.Wait()
}

func (np *NodePool) GetCandidates() []*Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	res := make([]*Node, len(np.candidates))
	copy(res, np.candidates)
	return res
}

func (np *NodePool) GetNodeByID(id string) *Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	for _, n := range np.candidates {
		if n.ID == id || n.IP == id {
			return n
		}
	}
	return nil
}

func (np *NodePool) SelectBest() *Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	// 1. Prefer reachable nodes with measured latency
	var reachable []*Node
	for _, n := range np.candidates {
		if !np.blacklist.IsBlacklisted(n.ID) {
			if n.LatencyMs > 0 {
				reachable = append(reachable, n)
			}
		}
	}

	if len(reachable) > 0 {
		sort.Slice(reachable, func(i, j int) bool {
			return reachable[i].LatencyMs < reachable[j].LatencyMs
		})
		return reachable[0]
	}

	// 2. Otherwise pick top candidate not blacklisted
	for _, n := range np.candidates {
		if !np.blacklist.IsBlacklisted(n.ID) {
			return n
		}
	}
	return nil
}

func (np *NodePool) Blacklist() *BlacklistManager {
	return np.blacklist
}

func (np *NodePool) Status() (string, string, time.Time, int) {
	np.mu.RLock()
	defer np.mu.RUnlock()

	return np.lastStatus, np.lastSource, np.lastUpdated, len(np.candidates)
}
