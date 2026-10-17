package fixture

import "reflect"

// Options holds settings.
type Options struct {
	Host string
}

// Default fills the empty string fields of o with "localhost".
func Default(o Options) {
	v := reflect.ValueOf(o)
	for i := range v.NumField() {
		if v.Field(i).String() == "" {
			v.Field(i).SetString("localhost")
		}
	}
}
