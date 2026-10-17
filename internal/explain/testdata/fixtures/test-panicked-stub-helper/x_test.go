package fixture

import "testing"

// catchPanic reports a panic at the line where it happened, because it
// calls t.Helper.
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

func TestBurn(t *testing.T) {
	defer catchPanic(t)
	if got := Burn(1, 100); got != 0.01 {
		t.Errorf("Burn(1, 100) = %v, want 0.01", got)
	}
}
