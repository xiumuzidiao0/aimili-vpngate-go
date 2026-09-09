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
	SelectTunnel(port int, boundIDs []string, policy PortPolicy, intervalSec int) *tunnel.Tunnel
}

type DefaultScheduler struct {
	pool    *tunnel.Pool
	counter atomic.Uint64
}

func NewScheduler(pool *tunnel.Pool) *DefaultScheduler {
	return &DefaultScheduler{pool: pool}
}

func (s *DefaultScheduler) SelectTunnel(port int, boundIDs []string, policy PortPolicy, intervalSec int) *tunnel.Tunnel {
	if s.pool == nil {
		return nil
	}

	healthy := s.pool.GetHealthyTunnels(boundIDs)
	if len(healthy) == 0 {
		// Fall back to any healthy tunnel in the pool
		healthy = s.pool.GetHealthyTunnels(nil)
		if len(healthy) == 0 {
			return nil
		}
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
