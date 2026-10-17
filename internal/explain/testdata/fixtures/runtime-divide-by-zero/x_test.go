package fixture

import "testing"

func TestMean(t *testing.T) {
	if got := Mean(nil); got != 0 {
		t.Errorf("Mean(nil) = %d, want 0", got)
	}
}
