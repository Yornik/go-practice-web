package fixture

func Evens(limit int) func(yield func(int)) {
	return func(yield func(int)) {
		for i := 0; i < limit; i += 2 {
			yield(i)
		}
	}
}

func Sum(limit int) int {
	total := 0
	for n := range Evens(limit) {
		total += n
	}
	return total
}
