package fixture

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrent(t *testing.T) {
	var c Counter
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				c.Inc()
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("%d goroutines doing %d calls each did not finish within %v (deadlock?)", 4, 10, 100*time.Millisecond)
	}
}
