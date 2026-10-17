package fixture_test

import (
	"testing"

	fixture "example.com/fixture"
)

func TestSum(t *testing.T) {
	if got := fixture.Sum([]int{1, 2}); got != 3 {
		t.Errorf("Sum = %d, want 3", got)
	}
}
