package fixture

import "math"

func Pages(items, perPage int) int {
	return int(math.Ceil(float64(items / perPage)))
}
