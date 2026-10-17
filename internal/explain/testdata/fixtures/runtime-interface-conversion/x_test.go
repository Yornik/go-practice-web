package fixture

import "testing"

func TestPort(t *testing.T) {
	if got := Port(map[string]any{"port": "8080"}); got != 8080 {
		t.Errorf("Port = %d, want 8080", got)
	}
}
