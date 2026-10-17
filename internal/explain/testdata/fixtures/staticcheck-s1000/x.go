package fixture

func Next(results <-chan int) int {
	select {
	case r := <-results:
		return r
	}
}
