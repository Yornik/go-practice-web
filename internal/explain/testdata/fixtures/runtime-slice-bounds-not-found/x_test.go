package fixture

import "testing"

func TestBetween(t *testing.T) {
	if got := Between("(abc", '(', ')'); got != "" {
		t.Errorf("Between = %q, want empty", got)
	}
}
