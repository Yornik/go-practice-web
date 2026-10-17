package fixture

import "testing"

func TestSum(t *testing.T) {
	if got := Sum([]int{1, 2, 3, 4}); got != 10 {
		t.Errorf("Sum = %d, want 10", got)
	}
}
