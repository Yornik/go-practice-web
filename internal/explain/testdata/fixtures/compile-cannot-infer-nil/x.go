package fixture

func Count[K comparable, V any](m map[K]V) int {
	return len(m)
}

func Empty() int {
	return Count(nil)
}
