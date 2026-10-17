package fixture

type Number interface {
	~int | ~float64
}

func Total[T Number](xs []T) T {
	var sum T
	for _, x := range xs {
		sum += x
	}
	return sum
}

func Words() string {
	return Total([]string{"a", "b"})
}
