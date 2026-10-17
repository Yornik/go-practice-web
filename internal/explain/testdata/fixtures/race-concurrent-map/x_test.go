package fixture

import "testing"

func TestStatus(t *testing.T) {
	got := Status([]string{"a", "b"})
	if got["a"] != 200 {
		t.Errorf("Status = %v, want a: 200", got)
	}
}
