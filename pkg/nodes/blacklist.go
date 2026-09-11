package nodes

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BlacklistEntry struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	Port      int       `json:"port,omitempty"`
	Protocol  string    `json:"protocol,omitempty"`
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
	if node == nil {
		return
	}
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	failCount := 1
	if existing, ok := bm.entries[node.ID]; ok {
		failCount = existing.FailCount + 1
	}

	if baseDuration <= 0 {
		baseDuration = 15 * time.Minute
	}

	// Mild backoff: 15m -> 30m -> 1h -> max 6h (avoid locking recoverable nodes excessively)
	multiplier := 1 << (failCount - 1)
	if multiplier > 24 {
		multiplier = 24
	}
	duration := baseDuration * time.Duration(multiplier)
	if duration > 6*time.Hour {
		duration = 6 * time.Hour
	}

	bm.entries[node.ID] = &BlacklistEntry{
		ID:        node.ID,
		IP:        node.IP,
		Port:      node.Port,
		Protocol:  node.Proto,
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

	cleanIP := ip
	port := 0
	if strings.Contains(ip, ":") {
		if h, pStr, err := net.SplitHostPort(ip); err == nil {
			cleanIP = h
			port, _ = strconv.Atoi(pStr)
		}
	}

	bm.entries[id] = &BlacklistEntry{
		ID:        id,
		IP:        cleanIP,
		Port:      port,
		Country:   country,
		Reason:    reason,
		MarkedAt:  now,
		Until:     now.Add(duration),
		FailCount: 1,
	}
	bm.saveLocked()
}

// ProbeAndRevive tests blacklisted nodes and auto-revives those that respond to TCP dial
func (bm *BlacklistManager) ProbeAndRevive(ctx context.Context, fallbackPortFinder func(nodeID, ip string) int) ([]*BlacklistEntry, error) {
	bm.mu.RLock()
	now := time.Now()
	var candidates []*BlacklistEntry
	for _, entry := range bm.entries {
		if entry != nil && entry.Until.After(now) {
			cp := *entry
			candidates = append(candidates, &cp)
		}
	}
	bm.mu.RUnlock()

	if len(candidates) == 0 {
		return nil, nil
	}

	var revived []*BlacklistEntry
	var revMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)

	for _, item := range candidates {
		wg.Add(1)
		go func(entry *BlacklistEntry) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			port := entry.Port
			if port <= 0 && fallbackPortFinder != nil {
				port = fallbackPortFinder(entry.ID, entry.IP)
			}
			if port <= 0 {
				port = 443
			}

			addr := net.JoinHostPort(entry.IP, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp", addr, 3500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				revMu.Lock()
				revived = append(revived, entry)
				revMu.Unlock()
			}
		}(item)
	}

	wg.Wait()

	if len(revived) > 0 {
		bm.mu.Lock()
		for _, r := range revived {
			delete(bm.entries, r.ID)
		}
		bm.saveLocked()
		bm.mu.Unlock()
	}

	return revived, nil
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
