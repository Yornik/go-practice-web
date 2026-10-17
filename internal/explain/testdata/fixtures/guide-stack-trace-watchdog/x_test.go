package fixture

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"
)

// guard stops the test binary when the test hangs, after it prints every
// goroutine, as the guard of the course's 08-ratelimit tests does. Inc
// never returns, so the timer always fires.
func guard(t *testing.T) func() {
	name := t.Name()
	watchdog := time.AfterFunc(100*time.Millisecond, func() {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		fmt.Fprintf(os.Stderr, "\n%s is still running after 100ms: a call is blocked.\n"+
			"Stacks of all goroutines:\n\n%s\n", name, stacks.String())
		os.Exit(2)
	})
	return func() { watchdog.Stop() }
}

func TestInc(t *testing.T) {
	t.Run("once", func(t *testing.T) {
		defer guard(t)()
		var c Counter
		c.Inc()
	})
}
