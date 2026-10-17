package fixture

import "time"

// Probe checks a service in the background and returns its status after
// a short wait.
func Probe() string {
	status := "unknown"
	go func() {
		status = "up"
	}()
	time.Sleep(10 * time.Millisecond)
	return status
}
