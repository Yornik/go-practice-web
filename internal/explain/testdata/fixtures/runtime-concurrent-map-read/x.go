package fixture

// Cache maps keys to values.
type Cache struct {
	m map[int]int
}

// NewCache returns an empty Cache.
func NewCache() *Cache {
	return &Cache{m: make(map[int]int)}
}

// Get returns the value of k.
func (c *Cache) Get(k int) int {
	return c.m[k]
}

// Set stores v under k.
func (c *Cache) Set(k, v int) {
	c.m[k] = v
}
