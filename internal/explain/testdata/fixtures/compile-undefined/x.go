package fixture

import "strconv"

func Total(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return n
}
