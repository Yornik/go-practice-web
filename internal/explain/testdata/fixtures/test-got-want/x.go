package fixture

// Mean returns the average of xs, or 0 for no values.
func Mean(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs)+1)
}
