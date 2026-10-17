package fixture

import "time"

func (d time.Duration) Millis() int64 {
	return d.Milliseconds()
}
