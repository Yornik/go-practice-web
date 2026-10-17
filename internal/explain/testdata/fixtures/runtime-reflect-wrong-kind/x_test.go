package fixture

import "testing"

func TestSum(t *testing.T) {
	if got := Sum(Stats{Hits: 2, Misses: 1, Name: "api"}); got != 3 {
		t.Errorf("Sum = %d, want 3", got)
	}
}
