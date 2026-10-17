package fixture

import "testing"

func TestResetNil(t *testing.T) {
	var p *int
	Reset(p)
}
