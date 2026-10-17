package fixture

func Countdown(n int) []int {
	var out []int
	while n > 0 {
		out = append(out, n)
		n--
	}
	return out
}
