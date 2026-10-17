package fixture

import "strings"

func Key(s string) string {
	strings.TrimSpace(s)
	return s
}
