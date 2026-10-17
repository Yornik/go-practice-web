package fixture

import "time"

// Options configures Poll. A zero Interval means 1 second.
type Options struct {
	Interval time.Duration
}

// Poll calls f n times, once per interval.
func Poll(o Options, n int, f func()) {
	t := time.NewTicker(o.Interval)
	defer t.Stop()
	for range n {
		<-t.C
		f()
	}
}
