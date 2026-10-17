package fixture

import "time"

func Scale(n int, unit time.Duration) time.Duration {
	return time.Duration(n) * unit
}

func First() time.Duration {
	return Scale(3)
}
