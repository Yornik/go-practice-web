package fixture

import "testing"

func TestPort(t *testing.T) {
	if _, err := Port("x"); err == nil {
		t.Error("want an error")
	}
}
