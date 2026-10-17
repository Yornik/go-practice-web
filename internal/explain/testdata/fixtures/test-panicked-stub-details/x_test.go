package fixture

import "testing"

func sum(t *testing.T, xs []int) int {
	t.Helper()
	defer func() {
		if v := recover(); v != nil {
			t.Fatalf("Sum panicked: %v\ninput: %v", v, xs)
		}
	}()
	return Sum(xs)
}

func TestSum(t *testing.T) {
	if got := sum(t, []int{1}); got != 1 {
		t.Errorf("Sum([1]) = %d, want 1", got)
	}
}
