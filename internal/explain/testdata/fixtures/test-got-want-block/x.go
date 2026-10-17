package fixture

import "strings"

// Words returns the words of s in lower case.
func Words(s string) []string {
	return strings.Fields(s)
}
