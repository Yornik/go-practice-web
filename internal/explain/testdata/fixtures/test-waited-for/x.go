package fixture

import "sync"

// RunAll runs every function in its own goroutine, all at the same time,
// and returns when all have returned.
func RunAll(fns []func()) {
	var wg sync.WaitGroup
	for _, f := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
		wg.Wait()
	}
}
