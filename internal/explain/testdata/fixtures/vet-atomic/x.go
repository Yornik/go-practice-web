package fixture

import "sync/atomic"

var requests int64

func Hit() {
	requests = atomic.AddInt64(&requests, 1)
}
