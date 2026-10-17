package fixture

import "runtime"

// Probe checks a service in the background and returns its status once
// the check is done.
func Probe() string {
	status := "unknown"
	n := runtime.NumGoroutine()
	go func() {
		status = "up"
	}()
	// Waiting until the goroutine has ended puts its write before the read
	// in time, but it creates no happens-before edge: the race detector
	// reports the read every time, with the write as the previous access.
	for runtime.NumGoroutine() > n {
		runtime.Gosched()
	}
	return status
}
