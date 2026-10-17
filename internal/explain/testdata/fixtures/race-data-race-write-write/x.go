package fixture

import "sync"

// Last records which of the names checked in last, 1 goroutine per name.
// Wait orders both writes before the read in return, but nothing orders the
// 2 writes: the race detector reports them whichever runs first.
func Last(names []string) string {
	last := ""
	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			last = n
		}()
	}
	wg.Wait()
	return last
}
