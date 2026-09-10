package nodes

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/stats"
)

type NodePool struct {
	cfg       *config.Config
	fetcher   *Fetcher
	snapshot  *SnapshotManager
	blacklist  *BlacklistManager
	enricher   *IPEnricher
	favorites  *FavoritesManager
	reputation *ReputationManager

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
	enricher := NewIPEnricher(cfg.DataDir)
	favorites := NewFavoritesManager(cfg.DataDir)
	reputation := NewReputationManager(cfg.DataDir)

	return &NodePool{
		cfg:        cfg,
		fetcher:    fetcher,
		snapshot:   sm,
		blacklist:  bm,
		enricher:   enricher,
		favorites:  favorites,
		reputation: reputation,
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

	// 后台并发测试全部候选节点的 TCP 连通性与真实延迟
	go np.ProbeNodes(context.Background(), filtered)

	// Async IP type classification (residential vs hosting) in background
	go np.enricher.EnrichNodes(context.Background(), filtered)

	return nil
}

func (np *NodePool) ProbeNodes(ctx context.Context, nodeList []*Node) {
	if len(nodeList) == 0 {
		return
	}

	stats.LogInfo("Probe", "开始对 %d 个候选节点执行全量连通性与延迟测速...", len(nodeList))

	var wg sync.WaitGroup
	concurrency := 24
	sem := make(chan struct{}, concurrency)

	for _, n := range nodeList {
		wg.Add(1)
		go func(target *Node) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			start := time.Now()
			addr := fmt.Sprintf("%s:%d", target.IP, target.Port)
			conn, err := net.DialTimeout("tcp", addr, 2500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				latency := int(time.Since(start).Milliseconds())
				if latency <= 0 {
					latency = 1
				}
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
	stats.LogInfo("Probe", "全量节点测速完成！")
}

func (np *NodePool) ProbeSpecificNodes(ctx context.Context, ids []string) []*Node {
	np.mu.RLock()
	var targets []*Node
	if len(ids) == 0 {
		targets = append(targets, np.candidates...)
	} else {
		idMap := make(map[string]bool)
		for _, id := range ids {
			idMap[id] = true
		}
		for _, n := range np.candidates {
			if idMap[n.ID] || idMap[n.IP] {
				targets = append(targets, n)
			}
		}
	}
	np.mu.RUnlock()

	np.ProbeNodes(ctx, targets)
	return targets
}

func (np *NodePool) GetCandidates() []*Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	res := make([]*Node, len(np.candidates))
	for i, n := range np.candidates {
		cp := *n
		cp.IsFavorite = np.favorites.IsFavorite(n.ID)
		if np.reputation != nil {
			cp.ReputationScore = np.reputation.GetScore(n.IP)
		} else {
			cp.ReputationScore = 60
		}
		res[i] = &cp
	}
	return res
}

func (np *NodePool) Favorites() *FavoritesManager {
	return np.favorites
}

func (np *NodePool) Reputation() *ReputationManager {
	return np.reputation
}

func (np *NodePool) GetNodeByID(id string) *Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	for _, n := range np.candidates {
		if n.ID == id || n.IP == id {
			cp := *n
			cp.IsFavorite = np.favorites.IsFavorite(n.ID)
			if np.reputation != nil {
				cp.ReputationScore = np.reputation.GetScore(n.IP)
			} else {
				cp.ReputationScore = 60
			}
			return &cp
		}
	}
	return nil
}

func (np *NodePool) SelectBestWithFilter(ipType string, countries []string, preferFavorites bool) *Node {
	np.mu.RLock()
	defer np.mu.RUnlock()

	countryMap := make(map[string]bool)
	for _, c := range countries {
		countryMap[strings.ToUpper(strings.TrimSpace(c))] = true
	}

	matchesFilter := func(n *Node) bool {
		if np.blacklist.IsBlacklisted(n.ID) {
			return false
		}
		if len(countryMap) > 0 && !countryMap[n.CountryShort] {
			return false
		}
		if ipType != "" && ipType != "all" && n.IPType != ipType {
			return false
		}
		return true
	}

	// 1. If preferFavorites is true, look for reachable favorites first
	if preferFavorites {
		var favs []*Node
		for _, n := range np.candidates {
			if np.favorites.IsFavorite(n.ID) && matchesFilter(n) && n.LatencyMs > 0 {
				favs = append(favs, n)
			}
		}
		if len(favs) > 0 {
			sort.Slice(favs, func(i, j int) bool {
				return favs[i].LatencyMs < favs[j].LatencyMs
			})
			return favs[0]
		}
	}

	// 2. Reachable nodes matching filter
	var reachable []*Node
	for _, n := range np.candidates {
		if matchesFilter(n) && n.LatencyMs > 0 {
			reachable = append(reachable, n)
		}
	}
	if len(reachable) > 0 {
		sort.Slice(reachable, func(i, j int) bool {
			scoreA := 60
			scoreB := 60
			if np.reputation != nil {
				scoreA = np.reputation.GetScore(reachable[i].IP)
				scoreB = np.reputation.GetScore(reachable[j].IP)
			}
			if (scoreA < 35) != (scoreB < 35) {
				return scoreA >= 35
			}
			if reachable[i].LatencyMs != reachable[j].LatencyMs {
				return reachable[i].LatencyMs < reachable[j].LatencyMs
			}
			return scoreA > scoreB
		})
		return reachable[0]
	}

	// 3. Fallback to any node matching filter
	for _, n := range np.candidates {
		if matchesFilter(n) {
			return n
		}
	}

	// 4. Ultimate fallback to standard SelectBest
	return np.selectBestLocked()
}

func (np *NodePool) selectBestLocked() *Node {
	var reachable []*Node
	for _, n := range np.candidates {
		if !np.blacklist.IsBlacklisted(n.ID) && n.LatencyMs > 0 {
			reachable = append(reachable, n)
		}
	}
	if len(reachable) > 0 {
		sort.Slice(reachable, func(i, j int) bool {
			scoreA := 60
			scoreB := 60
			if np.reputation != nil {
				scoreA = np.reputation.GetScore(reachable[i].IP)
				scoreB = np.reputation.GetScore(reachable[j].IP)
			}
			if (scoreA < 35) != (scoreB < 35) {
				return scoreA >= 35
			}
			if reachable[i].LatencyMs != reachable[j].LatencyMs {
				return reachable[i].LatencyMs < reachable[j].LatencyMs
			}
			return scoreA > scoreB
		})
		return reachable[0]
	}
	for _, n := range np.candidates {
		if !np.blacklist.IsBlacklisted(n.ID) {
			return n
		}
	}
	return nil
}

func (np *NodePool) SelectBest() *Node {
	np.mu.RLock()
	defer np.mu.RUnlock()
	return np.selectBestLocked()
}

func (np *NodePool) Blacklist() *BlacklistManager {
	return np.blacklist
}

func (np *NodePool) Status() (string, string, time.Time, int) {
	np.mu.RLock()
	defer np.mu.RUnlock()

	return np.lastStatus, np.lastSource, np.lastUpdated, len(np.candidates)
}
