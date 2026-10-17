package fixture

import (
	"runtime"
	"testing"
	"time"
)

func TestFirstLeak(t *testing.T) {
	n := runtime.NumGoroutine()
	First([]string{"a x", "b x"}, "x")
	deadline := time.Now().Add(100 * time.Millisecond)
	for runtime.NumGoroutine() > n {
		if time.Now().After(deadline) {
			t.Errorf("%d goroutine(s) still running %v after First returned, want 0: "+
				"a goroutine that is blocked on a channel after First stopped early never ends",
				runtime.NumGoroutine()-n, 100*time.Millisecond)
			return
		}
		time.Sleep(time.Millisecond)
	}
}
