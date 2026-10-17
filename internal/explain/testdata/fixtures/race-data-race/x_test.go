package fixture

import "testing"

func TestHits(t *testing.T) {
	if got := Hits(100); got != 100 {
		t.Errorf("Hits(100) = %d, want 100", got)
	}
}
