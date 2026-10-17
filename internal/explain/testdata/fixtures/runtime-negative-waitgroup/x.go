package fixture

import (
	"sync"
	"time"
)

// Total adds up xs in a goroutine.
func Total(xs []int) int {
	total := 0
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, x := range xs {
			total += x
		}
		wg.Done()
		cleanup()
	}()
	wg.Wait()
	return total
}

// cleanup takes a while, so Wait has returned before the deferred Done.
func cleanup() {
	time.Sleep(50 * time.Millisecond)
}
