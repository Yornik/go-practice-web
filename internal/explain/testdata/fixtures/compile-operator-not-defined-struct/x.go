package fixture

import "time"

type Window struct {
	Start time.Time
	End   time.Time
}

func Before(a, b Window) bool {
	return a < b
}
