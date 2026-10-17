package fixture

import "sync"

// Counter is safe for concurrent use.
type Counter struct {
	mu sync.Mutex
	n  int
}

// Inc adds 1 to the count.
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	c.mu.Unlock()
}
