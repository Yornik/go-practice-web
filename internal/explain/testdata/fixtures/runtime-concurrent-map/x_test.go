package fixture

import (
	"runtime"
	"testing"
)

func TestCount(t *testing.T) {
	// The runtime notices the conflict only when 2 writes overlap in
	// time, so run on several threads and repeat until it does.
	runtime.GOMAXPROCS(4)
	words := make([]string, 1000)
	for i := range words {
		words[i] = "w"
	}
	for {
		if got := Count(words); got["w"] > len(words) {
			t.Fatalf("count %d", got["w"])
		}
	}
}
