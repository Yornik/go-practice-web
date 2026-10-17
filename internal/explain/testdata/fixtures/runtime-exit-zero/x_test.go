package fixture

import "testing"

func TestPortZero(t *testing.T) {
	if got := Port("0"); got != 0 {
		t.Errorf("Port(%q) = %d, want 0", "0", got)
	}
}
