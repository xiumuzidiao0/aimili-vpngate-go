package stats

import (
	"fmt"
	"sync"
	"time"
)

type LogLevel string

const (
	LogLevelInfo    LogLevel = "INFO"
	LogLevelWarning LogLevel = "WARNING"
	LogLevelError   LogLevel = "ERROR"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Module    string    `json:"module"`
	Message   string    `json:"message"`
}

type RingLog struct {
	mu          sync.RWMutex
	capacity    int
	entries     []LogEntry
	subscribers map[chan LogEntry]struct{}
}

var globalRingLog *RingLog

func InitRingLog(capacity int) *RingLog {
	if capacity <= 0 {
		capacity = 500
	}
	r := &RingLog{
		capacity:    capacity,
		entries:     make([]LogEntry, 0, capacity),
		subscribers: make(map[chan LogEntry]struct{}),
	}
	globalRingLog = r
	return r
}

func GetRingLog() *RingLog {
	if globalRingLog == nil {
		return InitRingLog(500)
	}
	return globalRingLog
}

func (r *RingLog) Log(level LogLevel, module, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Module:    module,
		Message:   msg,
	}

	// Print to stdout as well
	fmt.Printf("[%s] [%s] [%s] %s\n", entry.Timestamp.Format("15:04:05"), entry.Level, entry.Module, entry.Message)

	r.mu.Lock()
	if len(r.entries) >= r.capacity {
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, entry)

	// Broadcast to active SSE/WebSocket subscribers
	for ch := range r.subscribers {
		select {
		case ch <- entry:
		default:
			// Non-blocking if subscriber buffer is full
		}
	}
	r.mu.Unlock()
}

func (r *RingLog) Recent(limit int) []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := len(r.entries)
	if limit <= 0 || limit > n {
		limit = n
	}
	res := make([]LogEntry, limit)
	copy(res, r.entries[n-limit:])
	return res
}

func (r *RingLog) Subscribe() chan LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan LogEntry, 64)
	r.subscribers[ch] = struct{}{}
	return ch
}

func (r *RingLog) Unsubscribe(ch chan LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subscribers, ch)
	close(ch)
}

func LogInfo(module, format string, args ...any) {
	GetRingLog().Log(LogLevelInfo, module, format, args...)
}

func LogWarn(module, format string, args ...any) {
	GetRingLog().Log(LogLevelWarning, module, format, args...)
}

func LogError(module, format string, args ...any) {
	GetRingLog().Log(LogLevelError, module, format, args...)
}
