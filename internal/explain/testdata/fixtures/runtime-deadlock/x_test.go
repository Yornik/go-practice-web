package fixture

import "testing"

func TestDoubles(t *testing.T) {
	if got := Doubles([]int{1, 2}); len(got) != 2 {
		t.Errorf("Doubles = %v, want 2 values", got)
	}
}
