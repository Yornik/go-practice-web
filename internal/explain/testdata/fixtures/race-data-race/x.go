package fixture

import "sync"

// Hits counts n concurrent hits.
func Hits(n int) int {
	hits := 0
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hits++
		}()
	}
	wg.Wait()
	return hits
}
