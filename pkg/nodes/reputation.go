package nodes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type NodeReputation struct {
	NodeID          string    `json:"node_id"`
	IP              string    `json:"ip"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	ConnectCount    int       `json:"connect_count"`
	SuccessCount    int       `json:"success_count"`
	FailCount       int       `json:"fail_count"`
	TotalUptimeSec  int64     `json:"total_uptime_sec"`
	LastConnectedAt time.Time `json:"last_connected_at"`
	LastFailedAt    time.Time `json:"last_failed_at"`
	Score           int       `json:"score"` // 0 ~ 100 信誉分, 初始默认 60
}

type ReputationManager struct {
	filePath string
	mu       sync.RWMutex
	records  map[string]*NodeReputation // key: IP
}

func NewReputationManager(dataDir string) *ReputationManager {
	rm := &ReputationManager{
		filePath: filepath.Join(dataDir, "node_reputation.json"),
		records:  make(map[string]*NodeReputation),
	}
	rm.load()
	return rm
}

func (rm *ReputationManager) load() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	data, err := os.ReadFile(rm.filePath)
	if err != nil {
		return
	}

	var raw map[string]*NodeReputation
	if err := json.Unmarshal(data, &raw); err == nil {
		rm.records = raw
	}
}

func (rm *ReputationManager) saveLocked() {
	data, err := json.MarshalIndent(rm.records, "", "  ")
	if err != nil {
		return
	}
	tmp := rm.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err == nil {
		_ = os.Chmod(tmp, 0600)
		_ = os.Rename(tmp, rm.filePath)
	}
}

func (rm *ReputationManager) GetOrCreateLocked(ip, nodeID string) *NodeReputation {
	now := time.Now()
	rec, ok := rm.records[ip]
	if !ok {
		rec = &NodeReputation{
			NodeID:    nodeID,
			IP:        ip,
			FirstSeen: now,
			LastSeen:  now,
			Score:     60, // 初始基准分
		}
		rm.records[ip] = rec
	}
	rec.LastSeen = now
	return rec
}

func (rm *ReputationManager) GetScore(ip string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rec, ok := rm.records[ip]; ok {
		return rec.Score
	}
	return 60
}

func (rm *ReputationManager) GetRecord(ip string) *NodeReputation {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rec, ok := rm.records[ip]; ok {
		cp := *rec
		return &cp
	}
	return nil
}

func (rm *ReputationManager) GetAllRecords() map[string]*NodeReputation {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	res := make(map[string]*NodeReputation, len(rm.records))
	for ip, r := range rm.records {
		cp := *r
		res[ip] = &cp
	}
	return res
}

func (rm *ReputationManager) RecordSuccess(ip, nodeID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rec := rm.GetOrCreateLocked(ip, nodeID)
	rec.ConnectCount++
	rec.SuccessCount++
	rec.LastConnectedAt = time.Now()

	// 成功连通奖分
	rec.Score += 5
	if rec.Score > 100 {
		rec.Score = 100
	}
	rm.saveLocked()
}

func (rm *ReputationManager) RecordUptime(ip, nodeID string, uptimeSec int64) {
	if uptimeSec <= 0 {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	rec := rm.GetOrCreateLocked(ip, nodeID)
	rec.TotalUptimeSec += uptimeSec

	// 在网超过 30 分钟额外加分
	bonus := int(uptimeSec / 1800)
	if bonus > 15 {
		bonus = 15
	}
	rec.Score += bonus
	if rec.Score > 100 {
		rec.Score = 100
	}
	rm.saveLocked()
}

func (rm *ReputationManager) RecordFail(ip, nodeID string, wasQuickDrop bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rec := rm.GetOrCreateLocked(ip, nodeID)
	rec.ConnectCount++
	rec.FailCount++
	rec.LastFailedAt = time.Now()

	if wasQuickDrop {
		// 连上后 3 分钟内异常断开闪退，重罚
		rec.Score -= 25
	} else {
		rec.Score -= 15
	}

	if rec.Score < 0 {
		rec.Score = 0
	}
	rm.saveLocked()
}
