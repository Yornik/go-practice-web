package fixture

import (
	"sync/atomic"
	"testing"
	"time"
)

// timeout is short here; the course tests wait 5s.
const timeout = 100 * time.Millisecond

// waitFor polls cond until it returns true. It fails the test if that
// takes longer than the timeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRunAllConcurrent(t *testing.T) {
	var running atomic.Int32
	gate := make(chan struct{})
	defer close(gate)
	f := func() {
		running.Add(1)
		<-gate
	}
	go RunAll([]func(){f, f})
	waitFor(t, "2 functions run at the same time", func() bool { return running.Load() == 2 })
}
