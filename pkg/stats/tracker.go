package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

type TrafficSnapshot struct {
	TotalUploadBytes   uint64 `json:"total_upload_bytes"`
	TotalDownloadBytes uint64 `json:"total_download_bytes"`
	UploadSpeedBps     uint64 `json:"upload_speed_bps"`
	DownloadSpeedBps   uint64 `json:"download_speed_bps"`
	ActiveConnections  int64  `json:"active_connections"`
}

type TrafficTracker struct {
	uploadTotal   atomic.Uint64
	downloadTotal atomic.Uint64
	activeConns   atomic.Int64

	mu            sync.RWMutex
	lastCheckTime time.Time
	lastUpload    uint64
	lastDownload  uint64
	uploadSpeed   uint64
	downloadSpeed uint64
}

var globalTracker = &TrafficTracker{
	lastCheckTime: time.Now(),
}

func GetTrafficTracker() *TrafficTracker {
	return globalTracker
}

func (t *TrafficTracker) AddUpload(n uint64) {
	t.uploadTotal.Add(n)
}

func (t *TrafficTracker) AddDownload(n uint64) {
	t.downloadTotal.Add(n)
}

func (t *TrafficTracker) IncConn() {
	t.activeConns.Add(1)
}

func (t *TrafficTracker) DecConn() {
	t.activeConns.Add(-1)
}

func (t *TrafficTracker) Tick() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	duration := now.Sub(t.lastCheckTime).Seconds()
	if duration <= 0 {
		return
	}

	currUp := t.uploadTotal.Load()
	currDown := t.downloadTotal.Load()

	deltaUp := currUp - t.lastUpload
	deltaDown := currDown - t.lastDownload

	t.uploadSpeed = uint64(float64(deltaUp) / duration)
	t.downloadSpeed = uint64(float64(deltaDown) / duration)

	t.lastUpload = currUp
	t.lastDownload = currDown
	t.lastCheckTime = now
}

func (t *TrafficTracker) Snapshot() TrafficSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TrafficSnapshot{
		TotalUploadBytes:   t.uploadTotal.Load(),
		TotalDownloadBytes: t.downloadTotal.Load(),
		UploadSpeedBps:     t.uploadSpeed,
		DownloadSpeedBps:   t.downloadSpeed,
		ActiveConnections:  t.activeConns.Load(),
	}
}

func StartTrafficTicker() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			globalTracker.Tick()
		}
	}()
}
