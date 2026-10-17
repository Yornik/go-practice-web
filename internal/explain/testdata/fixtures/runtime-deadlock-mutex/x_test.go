package fixture

import "testing"

func TestInc(t *testing.T) {
	var c Counter
	c.Inc()
	if got := c.Get(); got != 1 {
		t.Errorf("Get() = %d, want 1", got)
	}
}
