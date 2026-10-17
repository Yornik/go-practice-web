package fixture

import "strconv"

func Port(s string) int {
	n := strconv.Atoi(s)
	return n
}
