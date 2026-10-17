package fixture

import "testing"

func TestLast(t *testing.T) {
	if got := Last(nil); got != 0 {
		t.Errorf("Last(nil) = %d, want 0", got)
	}
}
