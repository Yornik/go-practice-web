package fixture

import "testing"

func TestStopTwice(t *testing.T) {
	p := NewPoller()
	p.Stop()
	p.Stop()
}
