package fixture

import "testing"

func TestEach(t *testing.T) { Each([]string{"a"}, func(string) {}) }
