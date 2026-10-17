package fixture

import "strings"

func Host(url string) string {
	return strings.TrimLeft(url, "https://")
}
