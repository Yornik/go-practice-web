package fixture

import "sync"

// Count counts the words, 1 goroutine per word.
func Count(words []string) map[string]int {
	counts := make(map[string]int)
	var wg sync.WaitGroup
	for _, w := range words {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counts[w]++
		}()
	}
	wg.Wait()
	return counts
}
