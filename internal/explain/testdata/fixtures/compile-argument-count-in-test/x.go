package fixture

// Sum adds the numbers.
func Sum(xs []int, limit int) int {
	total := 0
	for _, x := range xs {
		total += min(x, limit)
	}
	return total
}
