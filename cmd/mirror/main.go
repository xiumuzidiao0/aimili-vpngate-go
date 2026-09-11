package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
)

type MirrorMeta struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"`
	GeneratedAt   int64  `json:"generated_at"`
	GeneratedISO  string `json:"generated_at_iso"`
	RowCount      int    `json:"row_count"`
	ByteCount     int    `json:"byte_count"`
	SHA256        string `json:"sha256"`
}

func main() {
	cfg := config.LoadConfig()
	sm := nodes.NewSnapshotManager("data")
	fetcher := nodes.NewFetcher(cfg.ApiURL, cfg.MirrorURL, sm)

	fmt.Println("[Mirror] 正在抓取最新 VPNGate 节点数据...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := fetcher.FetchNodes(ctx)
	if err != nil {
		fmt.Printf("[Mirror] 抓取失败: %v\n", err)
		os.Exit(1)
	}

	parsed, err := nodes.ParseVPNGateCSV(result.Data, 0)
	if err != nil {
		fmt.Printf("[Mirror] 数据校验失败: %v\n", err)
		os.Exit(1)
	}

	// #nosec G301 -- mirror files are public data intended for HTTP distribution.
	_ = os.MkdirAll("mirror", 0755)
	// #nosec G306 -- mirror files are public data intended for HTTP distribution.
	if err := os.WriteFile("mirror/vpngate.csv", result.Data, 0644); err != nil {
		fmt.Printf("[Mirror] 写入 mirror/vpngate.csv 失败: %v\n", err)
		os.Exit(1)
	}

	sum := sha256.Sum256(result.Data)
	now := time.Now()
	meta := MirrorMeta{
		SchemaVersion: 1,
		Source:        result.Source,
		GeneratedAt:   now.Unix(),
		GeneratedISO:  now.UTC().Format(time.RFC3339),
		RowCount:      len(parsed),
		ByteCount:     len(result.Data),
		SHA256:        hex.EncodeToString(sum[:]),
	}

	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	// #nosec G306 -- mirror files are public data intended for HTTP distribution.
	_ = os.WriteFile("mirror/vpngate.meta.json", append(metaBytes, '\n'), 0644)

	fmt.Printf("[Mirror] 成功生成镜像！共 %d 个有效节点，文件大小: %d 字节，哈希: %s\n", len(parsed), len(result.Data), meta.SHA256[:12])
}
