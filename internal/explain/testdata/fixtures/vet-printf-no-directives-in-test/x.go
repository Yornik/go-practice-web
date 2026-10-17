package fixture

import (
	"fmt"
	"strconv"
)

func Port(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse port", err)
	}
	return n, nil
}
