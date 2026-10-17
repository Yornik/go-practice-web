package fixture

import "testing"

func TestGaps(t *testing.T) {
	if got := Gaps(2, 3); len(got) != 0 {
		t.Errorf("len(Gaps(2, 3)) = %d, want 0", len(got))
	}
}
