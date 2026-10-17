package fixture

import "reflect"

// Options holds settings.
type Options struct {
	Host string
}

// Set sets the string field name of *o to value.
func Set(o *Options, name, value string) {
	reflect.ValueOf(o).Elem().FieldByName(name).SetString(value)
}
