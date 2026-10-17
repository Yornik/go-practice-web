package fixture

func Largest[T any](xs []T) T {
	var best T
	for i, x := range xs {
		if i == 0 || x > best {
			best = x
		}
	}
	return best
}
