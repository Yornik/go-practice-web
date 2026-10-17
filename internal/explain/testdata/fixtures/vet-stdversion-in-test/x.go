package fixture

import "sync"

func Each(items []string, handle func(string)) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Go(func() { handle(item) })
	}
	wg.Wait()
}
