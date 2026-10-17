package fixture

type Set struct {
	items map[string]bool
}

func Count(s Set) int {
	n := 0
	for range s {
		n++
	}
	return n
}
