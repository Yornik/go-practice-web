package fixture

// Gaps returns a slice for the gaps between n points after skipping k.
func Gaps(n, k int) []float64 {
	return make([]float64, n-k-1)
}
