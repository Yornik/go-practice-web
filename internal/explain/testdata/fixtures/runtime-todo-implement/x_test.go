package fixture_test

import (
	"testing"

	"example.com/fixture"
)

func TestSum(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want int
	}{
		{"empty", nil, 0},
		{"two", []int{1, 2}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fixture.Sum(tc.in); got != tc.want {
				t.Errorf("Sum(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestNeverRuns(t *testing.T) {}
