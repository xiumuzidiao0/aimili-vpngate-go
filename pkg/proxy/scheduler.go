package proxy

import (
	"crypto/rand"
	"math/big"
	"sync/atomic"
	"time"

	"aimili-vpngate-go/pkg/tunnel"
)

type PortPolicy string

const (
	PolicyRoundRobin PortPolicy = "round_robin"
	PolicyRandom     PortPolicy = "random"
	PolicyInterval   PortPolicy = "interval"
)

type TunnelSelector interface {
	SelectTunnel(port int, boundIDs []string, boundGroupIDs []string, policy PortPolicy, intervalSec int) *tunnel.Tunnel
}

type DefaultScheduler struct {
	pool       *tunnel.Pool
	dynamicMgr *tunnel.DynamicGroupManager
	counter    atomic.Uint64
}

func NewScheduler(pool *tunnel.Pool, dm *tunnel.DynamicGroupManager) *DefaultScheduler {
	return &DefaultScheduler{
		pool:       pool,
		dynamicMgr: dm,
	}
}

func (s *DefaultScheduler) SelectTunnel(port int, boundIDs []string, boundGroupIDs []string, policy PortPolicy, intervalSec int) *tunnel.Tunnel {
	if s.pool == nil {
		return nil
	}

	seen := make(map[string]bool)
	var healthy []*tunnel.Tunnel

	// 1. Resolve tunnels from bound dynamic groups
	if s.dynamicMgr != nil && len(boundGroupIDs) > 0 {
		groupTunnels := s.dynamicMgr.GetTunnelsForGroups(boundGroupIDs)
		for _, t := range groupTunnels {
			if t.IsHealthy() && !seen[t.ID] {
				healthy = append(healthy, t)
				seen[t.ID] = true
			}
		}
	}

	// 2. Resolve tunnels from explicitly bound static IDs
	if len(boundIDs) > 0 {
		staticTunnels := s.pool.GetHealthyTunnels(boundIDs)
		for _, t := range staticTunnels {
			if t.IsHealthy() && !seen[t.ID] {
				healthy = append(healthy, t)
				seen[t.ID] = true
			}
		}
	}

	// Fallback: if no specific bound tunnels were chosen, or if the bound static/group IDs have no healthy tunnels currently,
	// automatically fallback to any healthy tunnel in the whole pool so proxy connections never black-hole
	if len(healthy) == 0 {
		healthy = s.pool.GetHealthyTunnels(nil)
		if len(healthy) == 0 {
			return nil
		}
	}

	// Filter by circuit breaker: prefer available (non-circuit-broken) tunnels
	var available []*tunnel.Tunnel
	for _, t := range healthy {
		if t.IsAvailable() {
			available = append(available, t)
		}
	}
	if len(available) > 0 {
		healthy = available
	}

	n := len(healthy)
	if n == 1 {
		return healthy[0]
	}

	switch policy {
	case PolicyRandom:
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err == nil {
			return healthy[int(nBig.Int64())]
		}
		return healthy[0]

	case PolicyInterval:
		if intervalSec <= 0 {
			intervalSec = 300 // default 5 minutes
		}
		slot := (time.Now().Unix() / int64(intervalSec)) % int64(n)
		return healthy[int(slot)]

	case PolicyRoundRobin:
		fallthrough
	default:
		idx := s.counter.Add(1) % uint64(n)
		return healthy[int(idx)]
	}
}
