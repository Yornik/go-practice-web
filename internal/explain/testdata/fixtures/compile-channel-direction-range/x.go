package fixture

func Consume(in chan<- string) int {
	n := 0
	for range in {
		n++
	}
	return n
}
