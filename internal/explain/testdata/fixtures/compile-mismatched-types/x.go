package fixture

import "time"

func Timeout(retries int) time.Duration {
	return time.Second * retries
}
