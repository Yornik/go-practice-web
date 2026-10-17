package fixture

import "testing"

func TestTotal(t *testing.T) {
	if got := Total([]int{1, 2}); got != 3 {
		t.Errorf("Total = %d, want 3", got)
	}
	// The goroutine of Total panics in its deferred Done after Total has
	// returned. Wait for that panic, which ends the test binary.
	select {}
}
