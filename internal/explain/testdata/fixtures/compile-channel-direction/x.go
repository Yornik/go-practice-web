package fixture

func Drain(ch <-chan int) int {
	n := 0
	for range ch {
		n++
	}
	close(ch)
	return n
}
