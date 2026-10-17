package fixture

import (
	"strconv"
	"time"
)

func Parse(p, t string) (int, time.Duration, error) {
	port, err := strconv.Atoi(p)
	timeout, err := time.ParseDuration(t)
	if err != nil {
		return 0, 0, err
	}
	return port, timeout, nil
}
