package fixture

func Index(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
}
