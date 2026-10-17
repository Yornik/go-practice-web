package fixture

import "time"

func Grace() time.Duration {
	return 1.5 * time.Second
}
