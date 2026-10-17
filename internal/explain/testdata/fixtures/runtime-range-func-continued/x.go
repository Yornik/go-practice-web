package fixture

import (
	"iter"
	"strings"
)

// Fields yields the space-separated fields of s.
func Fields(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, f := range strings.Fields(s) {
			yield(f)
		}
	}
}
