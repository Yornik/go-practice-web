package fixture

import "sync"

type Conn struct {
	mu   sync.Mutex
	open bool
}

func Open(conns []Conn) int {
	n := 0
	for _, c := range conns {
		if c.open {
			n++
		}
	}
	return n
}
