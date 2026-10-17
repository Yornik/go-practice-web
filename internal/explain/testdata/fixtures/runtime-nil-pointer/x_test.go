package fixture

import "testing"

func TestPort(t *testing.T) {
	s := &Server{}
	if got := s.Port(); got != 8080 {
		t.Errorf("Port() = %d, want 8080", got)
	}
}
