package fixture

import "strconv"

func Parse(s string) (error, int) {
	n, err := strconv.Atoi(s)
	return err, n
}
