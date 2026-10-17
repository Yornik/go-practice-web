package fixture

// Doubles returns 2*x for every x, computed in a goroutine.
func Doubles(xs []int) []int {
	ch := make(chan int)
	for _, x := range xs {
		ch <- 2 * x
	}
	close(ch)
	var out []int
	for d := range ch {
		out = append(out, d)
	}
	return out
}
