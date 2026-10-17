package fixture

import "testing"

func TestPortNumber(t *testing.T) {
	if got := Port("8080"); got != 8080 {
		t.Errorf("Port(%q) = %d, want 8080", "8080", got)
	}
}

// TestPortInvalid never reports a result, and TestPortEmpty never runs.
func TestPortInvalid(t *testing.T) {
	if got := Port("http"); got != 0 {
		t.Errorf("Port(%q) = %d, want 0", "http", got)
	}
}

func TestPortEmpty(t *testing.T) {
	Port("0")
}
