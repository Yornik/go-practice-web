package fixture

func Merge(a, b []string) []string {
	for _, s := range b {
		a = append(a, s)
	}
	return a
}
