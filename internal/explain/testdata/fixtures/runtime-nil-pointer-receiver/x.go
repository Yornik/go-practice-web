package fixture

import "errors"

// Counter counts events.
type Counter struct {
	name string
	n    int
}

// Open returns a new Counter, or an error for an empty name.
func Open(name string) (*Counter, error) {
	if name == "" {
		return nil, errors.New("empty name")
	}
	return &Counter{name: name}, nil
}

// Inc counts 1 event.
func (c *Counter) Inc() {
	c.n++
}
