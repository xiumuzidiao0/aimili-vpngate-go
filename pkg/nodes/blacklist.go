package nodes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type BlacklistEntry struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	Country   string    `json:"country"`
	Reason    string    `json:"reason"`
	MarkedAt  time.Time `json:"marked_at"`
	Until     time.Time `json:"until"`
	FailCount int       `json:"fail_count"`
}

type BlacklistManager struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]*BlacklistEntry
}

func NewBlacklistManager(dataDir string) *BlacklistManager {
	bm := &BlacklistManager{
		filePath: filepath.Join(dataDir, "blacklist.json"),
		entries:  make(map[string]*BlacklistEntry),
	}
	bm.load()
	return bm
}

func (bm *BlacklistManager) load() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	data, err := os.ReadFile(bm.filePath)
	if err != nil {
		return
	}

	var raw map[string]*BlacklistEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	now := time.Now()
	cleaned := make(map[string]*BlacklistEntry)
	for k, v := range raw {
		if v != nil && v.Until.After(now) {
			cleaned[k] = v
		}
	}
	bm.entries = cleaned
}

func (bm *BlacklistManager) saveLocked() {
	data, err := json.MarshalIndent(bm.entries, "", "  ")
	if err != nil {
		return
	}
	tmpFile := bm.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err == nil {
		_ = os.Rename(tmpFile, bm.filePath)
	}
}

func (bm *BlacklistManager) IsBlacklisted(nodeID string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entry, ok := bm.entries[nodeID]
	if !ok {
		return false
	}
	return entry.Until.After(time.Now())
}

func (bm *BlacklistManager) Mark(node *Node, reason string, baseDuration time.Duration) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	failCount := 1
	if existing, ok := bm.entries[node.ID]; ok {
		failCount = existing.FailCount + 1
	}

	// Exponential backoff: baseDuration * 1, 2, 4... max 24h
	multiplier := 1 << (failCount - 1)
	if multiplier > 48 {
		multiplier = 48
	}
	duration := baseDuration * time.Duration(multiplier)
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}

	bm.entries[node.ID] = &BlacklistEntry{
		ID:        node.ID,
		IP:        node.IP,
		Country:   node.CountryShort,
		Reason:    reason,
		MarkedAt:  now,
		Until:     now.Add(duration),
		FailCount: failCount,
	}

	bm.saveLocked()
}

func (bm *BlacklistManager) Remove(nodeID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	delete(bm.entries, nodeID)
	bm.saveLocked()
}

func (bm *BlacklistManager) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.entries = make(map[string]*BlacklistEntry)
	bm.saveLocked()
}

func (bm *BlacklistManager) MarkManual(id, ip, country, reason string, duration time.Duration) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	if reason == "" {
		reason = "用户手动屏蔽"
	}
	bm.entries[id] = &BlacklistEntry{
		ID:        id,
		IP:        ip,
		Country:   country,
		Reason:    reason,
		MarkedAt:  now,
		Until:     now.Add(duration),
		FailCount: 1,
	}
	bm.saveLocked()
}

func (bm *BlacklistManager) Count() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	now := time.Now()
	count := 0
	for _, v := range bm.entries {
		if v.Until.After(now) {
			count++
		}
	}
	return count
}

func (bm *BlacklistManager) List() []*BlacklistEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	now := time.Now()
	var list []*BlacklistEntry
	for _, v := range bm.entries {
		if v.Until.After(now) {
			list = append(list, v)
		}
	}
	return list
}
