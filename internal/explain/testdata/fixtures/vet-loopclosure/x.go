package fixture

import "sync"

func Run(tasks []func()) {
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task()
		}()
	}
	wg.Wait()
}
