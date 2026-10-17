package fixture

import "reflect"

// Reset sets the int that p points to to 0.
func Reset(p any) {
	reflect.ValueOf(p).Elem().SetInt(0)
}
