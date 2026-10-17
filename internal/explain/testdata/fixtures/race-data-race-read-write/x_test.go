package fixture

import "testing"

func TestProbe(t *testing.T) {
	if got := Probe(); got != "up" {
		t.Errorf("Probe() = %q, want up", got)
	}
}
