package fixture

import "testing"

func TestPrefix(t *testing.T) {
	if got := Prefix("abc", 5); got != "abc" {
		t.Errorf("Prefix(abc, 5) = %q, want abc", got)
	}
}
