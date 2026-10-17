package fixture

import "strconv"

func Double(s string) int {
	return 2 * strconv.Atoi(s)
}
