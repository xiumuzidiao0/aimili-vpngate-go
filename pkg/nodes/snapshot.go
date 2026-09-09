package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SnapshotMeta struct {
	Source      string    `json:"source"`
	CachedAt    time.Time `json:"cached_at"`
	RowCount    int       `json:"row_count"`
	ByteCount   int       `json:"byte_count"`
	SHA256      string    `json:"sha256"`
}

type SnapshotManager struct {
	cachePath string
	metaPath  string
}

func NewSnapshotManager(dataDir string) *SnapshotManager {
	return &SnapshotManager{
		cachePath: filepath.Join(dataDir, "vpngate_cache.csv"),
		metaPath:  filepath.Join(dataDir, "vpngate_cache.meta.json"),
	}
}

func (sm *SnapshotManager) Save(data []byte, source string, rowCount int) error {
	sum := sha256.Sum256(data)
	hashStr := hex.EncodeToString(sum[:])

	meta := SnapshotMeta{
		Source:    source,
		CachedAt:  time.Now(),
		RowCount:  rowCount,
		ByteCount: len(data),
		SHA256:    hashStr,
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tmpData := sm.cachePath + ".tmp"
	if err := os.WriteFile(tmpData, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpData, sm.cachePath); err != nil {
		return err
	}

	tmpMeta := sm.metaPath + ".tmp"
	if err := os.WriteFile(tmpMeta, metaBytes, 0644); err != nil {
		return err
	}
	_ = os.Rename(tmpMeta, sm.metaPath)

	return nil
}

func (sm *SnapshotManager) Load() ([]byte, *SnapshotMeta, error) {
	data, err := os.ReadFile(sm.cachePath)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot cache file not found: %w", err)
	}

	var meta SnapshotMeta
	if metaBytes, err := os.ReadFile(sm.metaPath); err == nil {
		_ = json.Unmarshal(metaBytes, &meta)
	}

	return data, &meta, nil
}
