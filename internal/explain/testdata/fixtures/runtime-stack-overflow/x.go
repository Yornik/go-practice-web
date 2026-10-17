package fixture

// Reach counts the services that name depends on, directly or not.
func Reach(deps map[string][]string, name string) int {
	n := 0
	for _, d := range deps[name] {
		n += 1 + Reach(deps, d)
	}
	return n
}
