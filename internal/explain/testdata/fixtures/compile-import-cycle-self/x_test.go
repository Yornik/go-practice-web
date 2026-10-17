package fixture

import (
	"testing"

	"example.com/fixture"
)

func TestUpper(t *testing.T) {
	if fixture.Upper("a") != "A" {
		t.Fail()
	}
}
