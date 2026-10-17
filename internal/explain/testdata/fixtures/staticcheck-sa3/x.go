package fixture

func Sum(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}
