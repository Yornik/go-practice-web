package fixture

import "testing"

func TestFieldsBreak(t *testing.T) {
	var first string
	for f := range Fields("a b c") {
		first = f
		break
	}
	if first != "a" {
		t.Errorf("first = %q, want a", first)
	}
}
