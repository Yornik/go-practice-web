package fixture

import "strings"

// Between returns the text between the first open and the first close
// after it.
func Between(s string, open, close byte) string {
	i := strings.IndexByte(s, open)
	j := strings.IndexByte(s, close)
	return s[i+1 : j]
}
