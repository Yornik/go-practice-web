package fixture

import (
	"runtime/debug"
	"testing"
)

func TestReachCycle(t *testing.T) {
	// A small limit makes the overflow quick; the default is 1 GB.
	debug.SetMaxStack(1 << 20)
	deps := map[string][]string{"api": {"db"}, "db": {"api"}}
	if got := Reach(deps, "api"); got != 2 {
		t.Errorf("Reach = %d, want 2", got)
	}
}
