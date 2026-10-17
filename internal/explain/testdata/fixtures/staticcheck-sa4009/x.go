package fixture

func Limit(n int) int {
	n = 100
	if n > 50 {
		return 50
	}
	return n
}
