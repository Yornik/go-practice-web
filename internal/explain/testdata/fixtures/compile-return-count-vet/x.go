package fixture

import "strconv"

func ParsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	return n, nil
}
