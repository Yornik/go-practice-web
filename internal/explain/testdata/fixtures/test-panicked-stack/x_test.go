package fixture

import (
	"runtime/debug"
	"testing"
)

func TestLast(t *testing.T) {
	defer func() {
		if v := recover(); v != nil {
			t.Fatalf("Last([1 2]) panicked: %v\n%s", v, debug.Stack())
		}
	}()
	if got := Last([]int{1, 2}); got != 2 {
		t.Errorf("Last([1 2]) = %d, want 2", got)
	}
}
