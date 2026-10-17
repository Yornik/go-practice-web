package fixture

import "reflect"

// Stats holds counters.
type Stats struct {
	Hits   int
	Misses int
	Name   string
}

// Sum adds up the fields of s.
func Sum(s Stats) int64 {
	v := reflect.ValueOf(s)
	var total int64
	for i := range v.NumField() {
		total += v.Field(i).Int()
	}
	return total
}
