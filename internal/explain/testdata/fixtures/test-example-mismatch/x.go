package fixture

import "strings"

// Title upper-cases the first letter of every word.
func Title(s string) string {
	return strings.ToUpper(s)
}
