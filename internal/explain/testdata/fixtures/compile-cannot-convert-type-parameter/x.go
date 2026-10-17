package fixture

func Mean[T any](xs []T) float64 {
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	return sum / float64(len(xs))
}
