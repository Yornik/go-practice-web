package fixture

import "testing"

func TestSetUnknown(t *testing.T) {
	var o Options
	Set(&o, "Port", "8080")
}
