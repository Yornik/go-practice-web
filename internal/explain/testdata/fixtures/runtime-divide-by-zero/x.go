package fixture

// Mean returns the average of xs, rounded toward zero.
func Mean(xs []int) int {
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum / len(xs)
}
