package tunnel

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"aimili-vpngate-go/pkg/nodes"
)

type TunnelStatus string

const (
	StatusConnecting TunnelStatus = "connecting"
	StatusConnected  TunnelStatus = "connected"
	StatusFailed     TunnelStatus = "failed"
	StatusStopped    TunnelStatus = "stopped"
)

type Tunnel struct {
	ID          string       `json:"id"`           // e.g. "tunnel-0", "tunnel-1"
	DevName     string       `json:"dev_name"`     // e.g. "tun0", "tun1"
	DevIndex    int          `json:"dev_index"`    // 0, 1, 2...
	Node        *nodes.Node  `json:"node"`         // VPNGate node information
	Status      TunnelStatus `json:"status"`       // connecting, connected, failed, stopped
	Message     string       `json:"message"`      // latest status description
	ConnectedAt time.Time    `json:"connected_at"` // handshake completed time
	Uptime      int64        `json:"uptime"`       // seconds online
	LatencyMs   int          `json:"latency_ms"`   // real-time probe latency

	// Internal lifecycle management
	mu         sync.RWMutex
	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	confPath   string
	authPath   string
}

func (t *Tunnel) GetStatus() TunnelStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status
}

func (t *Tunnel) IsHealthy() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status == StatusConnected
}

func (t *Tunnel) Snapshot() *Tunnel {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var uptime int64
	if t.Status == StatusConnected && !t.ConnectedAt.IsZero() {
		uptime = int64(time.Since(t.ConnectedAt).Seconds())
	}

	return &Tunnel{
		ID:          t.ID,
		DevName:     t.DevName,
		DevIndex:    t.DevIndex,
		Node:        t.Node,
		Status:      t.Status,
		Message:     t.Message,
		ConnectedAt: t.ConnectedAt,
		Uptime:      uptime,
		LatencyMs:   t.LatencyMs,
	}
}
