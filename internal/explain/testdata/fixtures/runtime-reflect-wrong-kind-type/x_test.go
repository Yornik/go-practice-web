package fixture

import "testing"

func TestNames(t *testing.T) {
	if got := Names(&Stats{}); len(got) != 2 {
		t.Errorf("Names = %v, want 2 names", got)
	}
}
