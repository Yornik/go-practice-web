package fixture

func Squares(n int) []int {
	out := make([]int, n)
	for i, v := range n {
		out[i] = v * v
	}
	return out
}
