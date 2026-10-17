package fixture

import "reflect"

// Options holds settings.
type Options struct {
	Port int
}

// Apply sets the field name of *o to value.
func Apply(o *Options, name string, value any) {
	f := reflect.ValueOf(o).Elem().FieldByName(name)
	f.Set(reflect.ValueOf(value))
}
