package fixture

import "fmt"

func Label[T fmt.Stringer](x *T) string {
	return x.String()
}
