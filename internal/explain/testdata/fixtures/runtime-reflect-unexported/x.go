package fixture

import "reflect"

// Options holds settings.
type Options struct {
	Host  string
	token string
}

// Clear sets every string field of *o to "".
func Clear(o *Options) {
	v := reflect.ValueOf(o).Elem()
	for i := range v.NumField() {
		v.Field(i).SetString("")
	}
}
