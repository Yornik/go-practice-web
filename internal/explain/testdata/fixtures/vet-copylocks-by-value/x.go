package fixture

import "sync"

type Counter struct {
	mu   sync.Mutex
	hits map[string]int
}

func (c Counter) Inc(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits[path]++
}
