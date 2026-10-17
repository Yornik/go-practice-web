package fixture

import "strconv"

func Port(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 128)
}
