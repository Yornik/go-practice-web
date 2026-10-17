package fixture

import "testing"

func TestPorts(t *testing.T) {
	if got := Ports([]string{"http", "80", "https"}); len(got) != 1 {
		t.Errorf("Ports() = %v, want 1 port", got)
	}
}
