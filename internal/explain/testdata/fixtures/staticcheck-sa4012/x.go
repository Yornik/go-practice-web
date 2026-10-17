package fixture

import "math"

func Valid(x float64) bool {
	return x != math.NaN()
}
