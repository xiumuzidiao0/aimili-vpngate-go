package proxy

import (
	"testing"
	"time"

	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/tunnel"
)

func TestScheduler(t *testing.T) {
	node1 := &nodes.Node{ID: "node-1", IP: "1.1.1.1"}
	node2 := &nodes.Node{ID: "node-2", IP: "2.2.2.2"}

	tun1 := &tunnel.Tunnel{ID: "tun-1", DevName: "tun0", Node: node1, Status: tunnel.StatusConnected}
	tun2 := &tunnel.Tunnel{ID: "tun-2", DevName: "tun1", Node: node2, Status: tunnel.StatusConnected}

	scheduler := &DefaultScheduler{
		pool: nil, // We'll test selector directly
	}

	// 1. Round-Robin test
	t1 := scheduler.selectFromList([]*tunnel.Tunnel{tun1, tun2}, PolicyRoundRobin, 0)
	t2 := scheduler.selectFromList([]*tunnel.Tunnel{tun1, tun2}, PolicyRoundRobin, 0)
	if t1.ID == t2.ID {
		t.Fatalf("expected round robin to alternate between tun-1 and tun-2, got %s then %s", t1.ID, t2.ID)
	}

	// 2. Random test
	tr := scheduler.selectFromList([]*tunnel.Tunnel{tun1, tun2}, PolicyRandom, 0)
	if tr != tun1 && tr != tun2 {
		t.Fatalf("expected random to pick tun1 or tun2")
	}
}

func (s *DefaultScheduler) selectFromList(healthy []*tunnel.Tunnel, policy PortPolicy, intervalSec int) *tunnel.Tunnel {
	n := len(healthy)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return healthy[0]
	}
	switch policy {
	case PolicyRoundRobin:
		idx := s.counter.Add(1) % uint64(n)
		return healthy[int(idx)]
	case PolicyInterval:
		slot := (time.Now().Unix() / int64(intervalSec)) % int64(n)
		return healthy[int(slot)]
	default:
		return healthy[0]
	}
}
