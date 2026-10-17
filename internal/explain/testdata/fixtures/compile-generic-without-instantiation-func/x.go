package fixture

func Largest[T int | float64](xs []T) T {
	var best T
	for _, x := range xs {
		best = max(best, x)
	}
	return best
}

var pick = Largest
