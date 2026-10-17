package fixture

func Longest(words []string) int {
	max := 0
	for _, w := range words {
		max = max(max, len(w))
	}
	return max
}
