package fixture

import "strings"

func SameHost(a, b string) bool {
	return strings.ToLower(a) == strings.ToLower(b)
}
