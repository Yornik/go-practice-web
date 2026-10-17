package fixture

import (
	"runtime"
	"testing"
	"time"
)

func TestFirstLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	First([]string{"a x", "b x"}, "x")
	deadline := time.Now().Add(100 * time.Millisecond)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			stacks := buf[:runtime.Stack(buf, true)]
			t.Fatalf("%d goroutines are still running %v after First returned, but only %d existed before First was called: "+
				"a goroutine started by First is stuck. All goroutines:\n%s",
				runtime.NumGoroutine(), 100*time.Millisecond, before, stacks)
		}
		time.Sleep(time.Millisecond)
	}
}
