package fixture

import "testing"

func TestApply(t *testing.T) {
	var o Options
	Apply(&o, "Port", "8080")
}
