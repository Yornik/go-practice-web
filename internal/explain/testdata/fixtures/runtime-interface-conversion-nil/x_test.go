package fixture

import "testing"

func TestPortMissing(t *testing.T) {
	if got := Port(map[string]any{}); got != 0 {
		t.Errorf("Port = %d, want 0", got)
	}
}
