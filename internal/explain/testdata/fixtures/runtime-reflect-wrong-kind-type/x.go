package fixture

import "reflect"

// Stats holds counters.
type Stats struct {
	Hits   int
	Misses int
}

// Names returns the field names of the struct s points to.
func Names(s *Stats) []string {
	t := reflect.TypeOf(s)
	var names []string
	for i := range t.NumField() {
		names = append(names, t.Field(i).Name)
	}
	return names
}
