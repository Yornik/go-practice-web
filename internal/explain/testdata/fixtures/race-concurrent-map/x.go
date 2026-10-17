package fixture

import "sync"

// Status checks every URL in its own goroutine and returns the status
// of each. Wait orders the writes before the caller reads the map, but
// nothing orders the writes: the race detector reports them every time.
func Status(urls []string) map[string]int {
	status := make(map[string]int)
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status[u] = 200
		}()
	}
	wg.Wait()
	return status
}
