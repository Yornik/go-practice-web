package fixture

import "time"

// Last records which of the names checked in last, 1 goroutine per name.
func Last(names []string) string {
	last := ""
	for _, n := range names {
		go func() {
			last = n
		}()
		time.Sleep(10 * time.Millisecond)
	}
	return last
}
