package fixture

import "sync"

func Process(items []string, handle func(string)) {
	var wg sync.WaitGroup
	for _, item := range items {
		go func() {
			wg.Add(1)
			defer wg.Done()
			handle(item)
		}()
	}
	wg.Wait()
}
