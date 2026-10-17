package fixture

func Zero[T any]() T {
	var z T
	return z
}

func Reset() int {
	n := Zero()
	return n
}
