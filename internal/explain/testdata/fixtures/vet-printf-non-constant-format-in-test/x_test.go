package fixture

import "testing"

func TestCheck(t *testing.T) {
	if Check("") == nil {
		t.Error("want an error")
	}
}
