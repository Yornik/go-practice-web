package fixture

import "sync"

// Total adds up xs in a goroutine.
func Total(xs []int) int {
	var wg sync.WaitGroup
	sums := make(chan int)
	wg.Add(1)
	go func() {
		defer wg.Done()
		total := 0
		for _, x := range xs {
			total += x
		}
		wg.Done()
		// The send waits until Wait has returned and Total receives, so
		// the deferred Done always comes after Wait.
		sums <- total
	}()
	wg.Wait()
	return <-sums
}
