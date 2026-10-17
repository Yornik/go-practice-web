package fixture

import "testing"

func TestAdd(t *testing.T) {
	var tl Tally
	tl.Add("a")
	if got := tl.counts["a"]; got != 1 {
		t.Errorf("counts[a] = %d, want 1", got)
	}
}
