package fixture

count := 0

func Next() int {
	count++
	return count
}
