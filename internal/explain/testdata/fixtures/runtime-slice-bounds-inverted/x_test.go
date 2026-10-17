package fixture

import "testing"

func TestBetween(t *testing.T) {
	if got := Between("a)b(c", '(', ')'); got != "" {
		t.Errorf("Between = %q, want empty", got)
	}
}
