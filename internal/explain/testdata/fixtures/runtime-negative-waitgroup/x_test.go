package fixture

import (
	"testing"
	"time"
)

func TestTotal(t *testing.T) {
	if got := Total([]int{1, 2}); got != 3 {
		t.Errorf("Total = %d, want 3", got)
	}
	// Give the goroutine time to run its deferred Done.
	time.Sleep(200 * time.Millisecond)
}
