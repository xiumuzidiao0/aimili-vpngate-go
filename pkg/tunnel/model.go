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
	Uptime        int64         `json:"uptime"`                  // seconds online
	LatencyMs     int           `json:"latency_ms"`              // real-time probe latency
	Unlock        *UnlockResult `json:"unlock,omitempty"`        // AI and Streaming unlock probe status
	CircuitBroken bool          `json:"circuit_broken"`          // true if in cooldown due to consecutive failures

	// Internal lifecycle management
	mu                 sync.RWMutex
	cmd                *exec.Cmd
	cancelFunc         context.CancelFunc
	confPath           string
	authPath           string
	consecutiveFails   int
	lastFailureTime    time.Time
	circuitBrokenUntil time.Time
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

func (t *Tunnel) IsAvailable() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.Status != StatusConnected {
		return false
	}
	if !t.circuitBrokenUntil.IsZero() && time.Now().Before(t.circuitBrokenUntil) {
		return false
	}
	return true
}

func (t *Tunnel) IsCircuitBroken() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return !t.circuitBrokenUntil.IsZero() && time.Now().Before(t.circuitBrokenUntil)
}

func (t *Tunnel) RecordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Sliding window: discard isolated failure older than 2 minutes
	if !t.lastFailureTime.IsZero() && time.Since(t.lastFailureTime) > 2*time.Minute {
		t.consecutiveFails = 0
	}

	t.consecutiveFails++
	t.lastFailureTime = time.Now()
	if t.consecutiveFails >= 3 {
		t.circuitBrokenUntil = time.Now().Add(45 * time.Second)
	}
}

func (t *Tunnel) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.consecutiveFails > 0 || !t.circuitBrokenUntil.IsZero() {
		t.consecutiveFails = 0
		t.circuitBrokenUntil = time.Time{}
	}
}

func (t *Tunnel) Snapshot() *Tunnel {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var uptime int64
	if t.Status == StatusConnected && !t.ConnectedAt.IsZero() {
		uptime = int64(time.Since(t.ConnectedAt).Seconds())
	}

	var unlockCopy *UnlockResult
	if t.Unlock != nil {
		cp := *t.Unlock
		unlockCopy = &cp
	}

	return &Tunnel{
		ID:            t.ID,
		DevName:       t.DevName,
		DevIndex:      t.DevIndex,
		Node:          t.Node,
		Status:        t.Status,
		Message:       t.Message,
		ConnectedAt:   t.ConnectedAt,
		Uptime:        uptime,
		LatencyMs:     t.LatencyMs,
		Unlock:        unlockCopy,
		CircuitBroken: !t.circuitBrokenUntil.IsZero() && time.Now().Before(t.circuitBrokenUntil),
	}
}
