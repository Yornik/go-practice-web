package fixture

func Total(xs []int) int {
	var sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum
}
