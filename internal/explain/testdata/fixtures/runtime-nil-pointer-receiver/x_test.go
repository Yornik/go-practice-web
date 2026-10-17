package fixture

import "testing"

func TestInc(t *testing.T) {
	c, _ := Open("")
	c.Inc()
}
