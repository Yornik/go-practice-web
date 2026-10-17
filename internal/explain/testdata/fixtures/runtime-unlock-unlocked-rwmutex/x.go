package fixture

import "sync"

// Registry maps names to addresses.
type Registry struct {
	mu    sync.RWMutex
	addrs map[string]string
}

// Lookup returns the address of name.
func (r *Registry) Lookup(name string) string {
	r.mu.RLock()
	defer r.mu.Unlock()
	return r.addrs[name]
}
