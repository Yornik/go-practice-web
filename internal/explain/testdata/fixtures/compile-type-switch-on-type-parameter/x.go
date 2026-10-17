package fixture

import "strconv"

func Format[T any](v T) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	}
	return "?"
}
