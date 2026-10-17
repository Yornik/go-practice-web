package fixture

import "testing"

func TestLast(t *testing.T) {
	if got := Last([]int{1, 2}); got != 2 {
		t.Errorf("Last([1 2]) = %d, want 2", got)
	}
}
