package fixture

import "testing"

func BenchmarkSum(b *testing.B) {
	b.N = 1000
	for i := 0; i < b.N; i++ {
		Sum([]int{1, 2, 3})
	}
}
