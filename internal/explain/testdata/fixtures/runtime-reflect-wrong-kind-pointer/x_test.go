package fixture

import "testing"

func TestFields(t *testing.T) {
	if got := Fields(&Stats{}); got != 2 {
		t.Errorf("Fields = %d, want 2", got)
	}
}
