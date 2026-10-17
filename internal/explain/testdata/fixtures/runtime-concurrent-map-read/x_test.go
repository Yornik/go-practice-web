package fixture

import (
	"runtime"
	"testing"
)

func TestGetWhileSet(t *testing.T) {
	// The runtime notices the conflict only when a read and a write
	// overlap in time, so run on several threads and repeat until it does.
	runtime.GOMAXPROCS(4)
	c := NewCache()
	go func() {
		for i := 0; ; i++ {
			c.Set(i%1000, i)
		}
	}()
	for i := 0; ; i++ {
		c.Get(i % 1000)
	}
}
