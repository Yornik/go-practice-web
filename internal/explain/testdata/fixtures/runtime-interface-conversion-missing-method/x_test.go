package fixture

import (
	"strings"
	"testing"
)

func TestDrain(t *testing.T) {
	if err := Drain(strings.NewReader("a")); err != nil {
		t.Errorf("Drain = %v, want nil", err)
	}
}
