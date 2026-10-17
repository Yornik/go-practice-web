package fixture

func Count(lines []string) int {
	count := 0
	for _, line := range lines {
		if line != "" {
			count := count + 1
		}
	}
	return count
}
