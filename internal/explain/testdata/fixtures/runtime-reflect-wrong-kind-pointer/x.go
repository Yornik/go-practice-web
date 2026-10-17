package fixture

import "reflect"

// Stats holds counters.
type Stats struct {
	Hits   int
	Misses int
}

// Fields returns the number of fields of the struct s points to.
func Fields(s *Stats) int {
	return reflect.ValueOf(s).NumField()
}
