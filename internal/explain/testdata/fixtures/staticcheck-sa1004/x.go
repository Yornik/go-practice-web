package fixture

import "time"

func Backoff(attempt int) {
	time.Sleep(100)
}
