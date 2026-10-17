package fixture

func Reverse[T](xs []T) []T {
	out := make([]T, 0, len(xs))
	for i := len(xs) - 1; i >= 0; i-- {
		out = append(out, xs[i])
	}
	return out
}
