package fixture

import "time"

func Wait(timeoutSecs time.Duration) {
	time.Sleep(timeoutSecs)
}
