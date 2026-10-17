package fixture

import "testing"

func TestMissingPath(t *testing.T) {
	if got := MissingPath(nil); got != "" {
		t.Errorf("MissingPath(nil) = %q", got)
	}
}
