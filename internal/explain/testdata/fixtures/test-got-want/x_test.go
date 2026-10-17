package fixture

import "testing"

func TestMean(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty", nil, 0},
		{"two values", []float64{1, 3}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Mean(tc.in); got != tc.want {
				t.Errorf("Mean(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
