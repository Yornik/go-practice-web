package fixture

import (
	"errors"
	"testing"
)

func TestLimitValidate(t *testing.T) {
	cases := []struct {
		name string
		l    Limit
	}{
		{"zero limit", Limit{}},
	}
	for _, c := range cases {
		if err := c.l.Validate(); !errors.Is(err, ErrBadLimit) {
			t.Errorf("%s: Validate() = %v, want an error that wraps ErrBadLimit", c.name, err)
		}
	}
}
