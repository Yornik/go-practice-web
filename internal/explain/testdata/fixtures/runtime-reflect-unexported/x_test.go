package fixture

import "testing"

func TestClear(t *testing.T) {
	o := Options{Host: "db", token: "x"}
	Clear(&o)
}
