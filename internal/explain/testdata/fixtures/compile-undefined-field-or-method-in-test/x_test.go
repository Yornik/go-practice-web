package fixture_test

import (
	"testing"

	fixture "example.com/fixture"
)

func TestMessage(t *testing.T) {
	var r fixture.Message
	if r.Msg != "" {
		t.Errorf("Msg = %q, want empty", r.Msg)
	}
}
