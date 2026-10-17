package fixture

import "time"

func Day(s string) (time.Time, error) {
	return time.Parse("2006-02-01", s)
}
