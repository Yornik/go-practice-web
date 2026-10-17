package fixture

import "testing"

func TestSum(t *testing.T) {
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("Sum(nil) panicked with %q, want 0", v)
		}
	}()
	Sum(nil)
}
