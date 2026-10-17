package fixture

import "testing"

func TestHosts(t *testing.T) {
	if len(Hosts()) == 0 {
		t.Error("Hosts() is empty")
	}
}
