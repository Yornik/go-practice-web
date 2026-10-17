package fixture

import "testing"

func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

func TestAdd(t *testing.T) {
	defer catchPanic(t)
	var tl Tally
	tl.Add("a")
}
