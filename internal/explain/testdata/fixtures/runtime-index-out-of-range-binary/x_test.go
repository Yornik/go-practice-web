package fixture

import "testing"

func TestLength(t *testing.T) {
	if got := Length([]byte{0, 1}); got != 0 {
		t.Errorf("Length = %d, want 0", got)
	}
}
