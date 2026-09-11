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
	Source    string    `json:"source"`
	CachedAt  time.Time `json:"cached_at"`
	RowCount  int       `json:"row_count"`
	ByteCount int       `json:"byte_count"`
	SHA256    string    `json:"sha256"`
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
	if err := os.WriteFile(tmpData, data, 0600); err != nil {
		return err
	}
	_ = os.Chmod(tmpData, 0600)
	if err := os.Rename(tmpData, sm.cachePath); err != nil {
		return err
	}

	tmpMeta := sm.metaPath + ".tmp"
	if err := os.WriteFile(tmpMeta, metaBytes, 0600); err != nil {
		return err
	}
	_ = os.Chmod(tmpMeta, 0600)
	_ = os.Rename(tmpMeta, sm.metaPath)

	return nil
}

func (sm *SnapshotManager) Load() ([]byte, *SnapshotMeta, error) {
	data, err := os.ReadFile(sm.cachePath)
	if err == nil && len(data) > 0 {
		var meta SnapshotMeta
		if metaBytes, err := os.ReadFile(sm.metaPath); err == nil {
			_ = json.Unmarshal(metaBytes, &meta)
		}
		return data, &meta, nil
	}

	// 尝试从程序内置或同目录镜像文件加载
	fallbackPaths := []string{
		"mirror/vpngate.csv",
		"/opt/aimilivpn/mirror/vpngate.csv",
		"../mirror/vpngate.csv",
	}
	for _, p := range fallbackPaths {
		// #nosec G304 -- fallback paths are fixed application-provided locations.
		if fbData, err := os.ReadFile(p); err == nil && len(fbData) > 0 {
			meta := SnapshotMeta{
				Source:    fmt.Sprintf("安装包内置镜像 (%s)", p),
				CachedAt:  time.Now(),
				RowCount:  0,
				ByteCount: len(fbData),
			}
			return fbData, &meta, nil
		}
	}

	return nil, nil, fmt.Errorf("snapshot cache file not found")
}
