package fixture

func Count(lines []string) int {
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}
	count := count * 2
	return count
}
