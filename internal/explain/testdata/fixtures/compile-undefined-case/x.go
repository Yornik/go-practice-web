package fixture

import "strings"

func HasError(line string) bool {
	return strings.contains(line, "error")
}
