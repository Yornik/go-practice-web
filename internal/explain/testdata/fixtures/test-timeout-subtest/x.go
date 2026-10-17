package fixture

// Sum adds up xs, each half in its own goroutine.
func Sum(xs []int) int {
	ch := make(chan int)
	half := len(xs) / 2
	go func() { ch <- add(xs[:half]) }()
	go func() { ch <- add(xs[half:]) }()
	return <-ch + <-ch + <-ch
}

func add(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// Watch reports every value of in, until in is closed.
func Watch(in <-chan int, report func(int)) {
	for v := range in {
		report(v)
	}
}
