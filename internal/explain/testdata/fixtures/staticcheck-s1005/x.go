package fixture

func Keys(m map[string]int) []string {
	var keys []string
	for k, _ := range m {
		keys = append(keys, k)
	}
	return keys
}
