package fixture

import "testing"

func Testparse(t *testing.T) {
	if Parse("ab") != 2 {
		t.Error("want 2")
	}
}
