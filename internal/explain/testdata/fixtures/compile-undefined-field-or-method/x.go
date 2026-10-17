package fixture

type Hit struct {
	Path  string
	Count int
}

func Total(hits []Hit) int {
	n := 0
	for _, h := range hits {
		n += h.count
	}
	return n
}
