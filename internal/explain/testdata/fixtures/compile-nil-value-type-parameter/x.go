package fixture

func First[T any](xs []T) (T, bool) {
	if len(xs) == 0 {
		return nil, false
	}
	return xs[0], true
}
