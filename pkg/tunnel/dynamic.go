package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
)

type DynamicGroup struct {
	ID              string    `json:"id"`               // e.g. "dg-1"
	Name            string    `json:"name"`             // e.g. "日本Top3住宅隧道组"
	Enabled         bool      `json:"enabled"`          // 是否启用自动自适应维护
	Country         string    `json:"country"`          // "JP", "US", or "" for 全部
	IPType          string    `json:"ip_type"`          // "residential", "hosting", "all"
	SortBy          string    `json:"sort_by"`          // "latency" (延迟优先), "speed" (带宽优先), "score" (评分优先)
	TargetCount     int       `json:"target_count"`     // 维持并发隧道数 (例如 3)
	IntervalMinutes int       `json:"interval_minutes"` // 重新评估与动态轮换周期 (分钟, 例如 15)
	ActiveTunnelIDs []string  `json:"active_tunnel_ids"`// 当前此组维护的隧道 ID 列表
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`// 上次重新评估并轮换的时间
	StatusText      string    `json:"status_text"`      // 状态摘要
}

type DynamicGroupManager struct {
	cfg       *config.Config
	pool      *Pool
	nodePool  *nodes.NodePool
	filePath  string
	mu        sync.RWMutex
	groups    map[string]*DynamicGroup
	evalMutex sync.Mutex
}

func NewDynamicGroupManager(cfg *config.Config, pool *Pool, np *nodes.NodePool) *DynamicGroupManager {
	m := &DynamicGroupManager{
		cfg:      cfg,
		pool:     pool,
		nodePool: np,
		filePath: filepath.Join(cfg.DataDir, "dynamic_groups.json"),
		groups:   make(map[string]*DynamicGroup),
	}
	m.load()
	return m
}

func (m *DynamicGroupManager) load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}

	var list []*DynamicGroup
	if err := json.Unmarshal(data, &list); err == nil {
		for _, g := range list {
			if g.ID != "" {
				m.groups[g.ID] = g
			}
		}
	}
}

func (m *DynamicGroupManager) saveLocked() {
	var list []*DynamicGroup
	for _, g := range m.groups {
		list = append(list, g)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}

	tmp := m.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err == nil {
		_ = os.Rename(tmp, m.filePath)
	}
}

func (m *DynamicGroupManager) ListGroups() []*DynamicGroup {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*DynamicGroup
	for _, g := range m.groups {
		cp := *g
		// Clone active IDs slice
		cp.ActiveTunnelIDs = make([]string, len(g.ActiveTunnelIDs))
		copy(cp.ActiveTunnelIDs, g.ActiveTunnelIDs)
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

func (m *DynamicGroupManager) GetGroup(id string) *DynamicGroup {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.groups[id]
	if !ok {
		return nil
	}
	cp := *g
	return &cp
}

func (m *DynamicGroupManager) SaveGroup(g *DynamicGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if g.ID == "" {
		g.ID = fmt.Sprintf("dg-%d", time.Now().UnixNano()%100000)
	}
	if g.TargetCount <= 0 {
		g.TargetCount = 3
	}
	if g.TargetCount > 16 {
		g.TargetCount = 16
	}
	if g.IntervalMinutes <= 0 {
		g.IntervalMinutes = 15
	}
	if g.SortBy == "" {
		g.SortBy = "latency"
	}
	if g.IPType == "" {
		g.IPType = "all"
	}

	m.groups[g.ID] = g
	m.saveLocked()

	stats.LogInfo("DynamicGroup", "保存动态自适应组 [%s] (%s): 目标数=%d, 策略=%s, 周期=%d分钟",
		g.ID, g.Name, g.TargetCount, g.SortBy, g.IntervalMinutes)
	return nil
}

func (m *DynamicGroupManager) DeleteGroup(id string) {
	m.mu.Lock()
	g, ok := m.groups[id]
	if ok {
		delete(m.groups, id)
		m.saveLocked()
	}
	m.mu.Unlock()

	if g != nil {
		// Stop any tunnels specifically owned by this deleted group
		for _, tunID := range g.ActiveTunnelIDs {
			_ = m.pool.StopTunnel(tunID)
		}
		stats.LogInfo("DynamicGroup", "已删除动态自适应组 [%s] 并释放其维护的出口隧道", id)
	}
}

// GetTunnelsForGroups returns all active healthy tunnels belonging to the specified dynamic groups
func (m *DynamicGroupManager) GetTunnelsForGroups(groupIDs []string) []*Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tunIDMap := make(map[string]bool)
	for _, gid := range groupIDs {
		if g, ok := m.groups[gid]; ok && g.Enabled {
			for _, tid := range g.ActiveTunnelIDs {
				tunIDMap[tid] = true
			}
		}
	}

	var available []*Tunnel
	var healthy []*Tunnel
	for tid := range tunIDMap {
		if t := m.pool.GetTunnel(tid); t != nil {
			if t.IsAvailable() {
				available = append(available, t)
			}
			if t.IsHealthy() {
				healthy = append(healthy, t)
			}
		}
	}
	if len(available) > 0 {
		return available
	}
	return healthy
}

// EvaluateGroup evaluates candidates, probes metrics, and dynamically rotates tunnels for a group
func (m *DynamicGroupManager) EvaluateGroup(ctx context.Context, g *DynamicGroup) {
	m.evalMutex.Lock()
	defer m.evalMutex.Unlock()

	allCandidates := m.nodePool.GetCandidates()
	if len(allCandidates) == 0 {
		return
	}

	// 1. Filter candidates by country & IP type
	var matched []*nodes.Node
	targetCountry := strings.ToUpper(strings.TrimSpace(g.Country))

	for _, n := range allCandidates {
		if m.nodePool.Blacklist().IsBlacklisted(n.ID) {
			continue
		}
		if targetCountry != "" && n.CountryShort != targetCountry {
			continue
		}
		if g.IPType != "" && g.IPType != "all" {
			if n.IPType != g.IPType {
				continue
			}
		}
		matched = append(matched, n)
	}

	if len(matched) == 0 {
		m.mu.Lock()
		g.StatusText = "无匹配候选节点"
		m.mu.Unlock()
		return
	}

	// 2. Probe top candidates to get fresh latency if sorting by latency or if unprobed
	probeLimit := 20
	if len(matched) < probeLimit {
		probeLimit = len(matched)
	}
	toProbe := matched[:probeLimit]
	m.nodePool.ProbeNodes(ctx, toProbe)

	// 3. Sort candidates according to g.SortBy
	sort.Slice(matched, func(i, j int) bool {
		a := matched[i]
		b := matched[j]

		switch g.SortBy {
		case "latency":
			// Reachable nodes first (latency > 0)
			aAvail := a.LatencyMs > 0
			bAvail := b.LatencyMs > 0
			if aAvail != bAvail {
				return aAvail
			}
			if aAvail && bAvail {
				if a.LatencyMs != b.LatencyMs {
					return a.LatencyMs < b.LatencyMs
				}
			}
			return a.Score > b.Score

		case "speed":
			if a.Speed != b.Speed {
				return a.Speed > b.Speed
			}
			return a.Score > b.Score

		case "score":
			fallthrough
		default:
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			if a.LatencyMs > 0 && b.LatencyMs > 0 {
				return a.LatencyMs < b.LatencyMs
			}
			return a.Ping < b.Ping
		}
	})

	// 4. Target count to maintain
	targetN := g.TargetCount
	if targetN <= 0 {
		targetN = 3
	}

	// 5. Clean up any stale/dead tunnels first
	m.pool.ReapStaleTunnels()

	m.mu.RLock()
	currentTunnelIDs := make([]string, len(g.ActiveTunnelIDs))
	copy(currentTunnelIDs, g.ActiveTunnelIDs)
	m.mu.RUnlock()

	activeTunnels := make(map[string]*Tunnel)
	for _, tid := range currentTunnelIDs {
		if t := m.pool.GetTunnel(tid); t != nil && t.IsHealthy() {
			activeTunnels[tid] = t
		} else {
			_ = m.pool.StopTunnel(tid)
		}
	}

	// 6. Identify existing healthy tunnels that match our group's criteria
	chosenTunnelIDs := make([]string, 0, targetN)
	usedNodeIDs := make(map[string]bool)

	// Keep existing healthy tunnels whose nodes are in the matched list (ranked best)
	for _, n := range matched {
		if len(chosenTunnelIDs) >= targetN {
			break
		}
		for tid, t := range activeTunnels {
			if t.Node != nil && (t.Node.ID == n.ID || t.Node.IP == n.IP) && t.IsHealthy() {
				chosenTunnelIDs = append(chosenTunnelIDs, tid)
				usedNodeIDs[n.ID] = true
				usedNodeIDs[n.IP] = true
				delete(activeTunnels, tid)
				break
			}
		}
	}

	// 7. Replenish / backfill any missing slots from the sorted matched list
	needed := targetN - len(chosenTunnelIDs)
	if needed > 0 {
		for _, n := range matched {
			if needed <= 0 {
				break
			}
			if usedNodeIDs[n.ID] || usedNodeIDs[n.IP] {
				continue
			}
			if m.nodePool.Blacklist().IsBlacklisted(n.ID) {
				continue
			}

			stats.LogInfo("DynamicGroup", "[%s] 递补启动新出口 (目标: %d, 仍缺: %d): 节点 %s (%s, 延迟: %dms, 带宽: %.1fMbps)",
				g.Name, targetN, needed, n.ID, n.CountryShort, n.LatencyMs, float64(n.Speed)/1000000.0)

			newTun, err := m.pool.StartTunnel(n)
			if err != nil || newTun == nil {
				stats.LogWarn("DynamicGroup", "[%s] 启动候选节点 %s 失败: %v，尝试下一个候选", g.Name, n.ID, err)
				m.nodePool.Blacklist().Mark(n, fmt.Sprintf("启动失败: %v", err), 600*time.Second)
				continue
			}

			// Wait briefly (up to 8s) for handshake completion or early exit
			connected := false
			for w := 0; w < 16; w++ {
				time.Sleep(500 * time.Millisecond)
				st := newTun.GetStatus()
				if st == StatusConnected {
					connected = true
					break
				}
				if st == StatusFailed || st == StatusStopped {
					break
				}
			}

			if connected {
				chosenTunnelIDs = append(chosenTunnelIDs, newTun.ID)
				usedNodeIDs[n.ID] = true
				usedNodeIDs[n.IP] = true
				needed--
			} else {
				stats.LogWarn("DynamicGroup", "[%s] 候选节点 %s 握手超时或失败，已自动释放并递补下一个候选...", g.Name, n.ID)
				_ = m.pool.StopTunnel(newTun.ID)
				m.nodePool.Blacklist().Mark(n, "动态组探测握手未通过", 900*time.Second)
			}
		}
	}

	// 8. Stop any leftover old tunnels that are no longer chosen
	for tid, oldTun := range activeTunnels {
		stats.LogInfo("DynamicGroup", "[%s] 平滑淘汰退役旧出口: %s (%s)", g.Name, tid, oldTun.DevName)
		_ = m.pool.StopTunnel(tid)
	}

	// 9. Update group status
	m.mu.Lock()
	g.ActiveTunnelIDs = chosenTunnelIDs
	g.LastEvaluatedAt = time.Now()
	if len(chosenTunnelIDs) >= g.TargetCount {
		g.StatusText = fmt.Sprintf("正常运行 (在网出口: %d/%d)", len(chosenTunnelIDs), g.TargetCount)
	} else {
		g.StatusText = fmt.Sprintf("部分就绪 (在网出口: %d/%d)", len(chosenTunnelIDs), g.TargetCount)
	}
	m.saveLocked()
	m.mu.Unlock()

	stats.LogInfo("DynamicGroup", "[%s] 动态评估完成，当前活跃出口数量: %d/%d", g.Name, len(chosenTunnelIDs), g.TargetCount)
}

// StartEvaluationLoop starts periodic health checking & evaluation for all dynamic groups
func (m *DynamicGroupManager) StartEvaluationLoop(ctx context.Context) {
	go func() {
		// Initial evaluation shortly after boot
		time.Sleep(3 * time.Second)
		m.evaluateAll(ctx)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.evaluateAll(ctx)
			}
		}
	}()
}

func (m *DynamicGroupManager) evaluateAll(ctx context.Context) {
	groups := m.ListGroups()
	now := time.Now()

	for _, g := range groups {
		if !g.Enabled {
			continue
		}

		interval := time.Duration(g.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = 15 * time.Minute
		}

		// Trigger evaluation if interval expired OR if any active tunnel is down
		needsEval := false
		if g.LastEvaluatedAt.IsZero() || now.Sub(g.LastEvaluatedAt) >= interval {
			needsEval = true
		} else {
			// Check if any tunnel dropped
			healthyCount := 0
			for _, tid := range g.ActiveTunnelIDs {
				if t := m.pool.GetTunnel(tid); t != nil && t.IsHealthy() {
					healthyCount++
				}
			}
			if healthyCount < g.TargetCount {
				needsEval = true
			}
		}

		if needsEval {
			m.EvaluateGroup(ctx, g)
		}
	}
}
