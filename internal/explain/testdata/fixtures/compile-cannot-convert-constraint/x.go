package fixture

func Weight[T int | string](x T) float64 {
	return float64(x) / 2
}
