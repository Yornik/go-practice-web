package fixture

import "testing"

func TestLabel(t *testing.T) {
	if got := Label(80); got != "port 80" {
		t.Errorf("Label(80) = %q", got)
	}
}
