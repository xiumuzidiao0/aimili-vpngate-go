package tunnel

import (
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	tun := &Tunnel{
		ID:      "tun-test",
		DevName: "tun0",
		Status:  StatusConnected,
	}

	// 1. Initial state: healthy and available
	if !tun.IsHealthy() {
		t.Fatalf("expected healthy tunnel")
	}
	if !tun.IsAvailable() {
		t.Fatalf("expected available tunnel initially")
	}
	if tun.IsCircuitBroken() {
		t.Fatalf("expected not circuit broken initially")
	}

	// 2. 1 failure: not broken yet
	tun.RecordFailure()
	if tun.IsCircuitBroken() {
		t.Fatalf("expected not circuit broken after 1 failure")
	}
	if !tun.IsAvailable() {
		t.Fatalf("expected still available after 1 failure")
	}

	// 3. 2nd failure: not broken yet
	tun.RecordFailure()
	if tun.IsCircuitBroken() {
		t.Fatalf("expected not circuit broken after 2 failures")
	}

	// 4. 3rd failure: triggers circuit breaker
	tun.RecordFailure()
	if !tun.IsCircuitBroken() {
		t.Fatalf("expected circuit broken after 3 consecutive failures")
	}
	if tun.IsAvailable() {
		t.Fatalf("expected NOT available when circuit broken")
	}

	// Snapshot reflects circuit broken state
	snap := tun.Snapshot()
	if !snap.CircuitBroken {
		t.Fatalf("expected snapshot to reflect circuit broken state")
	}

	// 5. Success resets circuit breaker
	tun.RecordSuccess()
	if tun.IsCircuitBroken() {
		t.Fatalf("expected circuit breaker reset after record success")
	}
	if !tun.IsAvailable() {
		t.Fatalf("expected available after record success")
	}

	// 6. Test expiration
	tun.RecordFailure()
	tun.RecordFailure()
	tun.RecordFailure()
	if !tun.IsCircuitBroken() {
		t.Fatalf("expected circuit broken again")
	}

	// Manually simulate past expiration time
	tun.mu.Lock()
	tun.circuitBrokenUntil = time.Now().Add(-1 * time.Second)
	tun.mu.Unlock()

	if tun.IsCircuitBroken() {
		t.Fatalf("expected circuit breaker expired")
	}
	if !tun.IsAvailable() {
		t.Fatalf("expected available after circuit breaker expired")
	}
}
