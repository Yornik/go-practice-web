package fixture

import "time"

// Status checks every URL in its own goroutine and returns the status
// of each.
func Status(urls []string) map[string]int {
	status := make(map[string]int)
	for _, u := range urls {
		go func() {
			status[u] = 200
		}()
		time.Sleep(10 * time.Millisecond)
	}
	return status
}
