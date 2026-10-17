package fixture

func Count(words []string) map[string]int {
	var counts map[string]int
	for _, w := range words {
		counts[w]++
	}
	return counts
}
