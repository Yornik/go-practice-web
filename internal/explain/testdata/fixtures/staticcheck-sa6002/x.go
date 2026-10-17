package fixture

import "sync"

var buffers = sync.Pool{New: func() any { return make([]byte, 0, 4096) }}

func Release(b []byte) {
	buffers.Put(b[:0])
}
