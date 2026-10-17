package fixture

import "time"

func Age(start time.Time) time.Duration {
	return time.Now().Sub(start)
}
