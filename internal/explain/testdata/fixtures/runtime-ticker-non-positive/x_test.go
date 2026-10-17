package fixture

import "testing"

func TestPollDefault(t *testing.T) {
	Poll(Options{}, 0, func() {})
}
