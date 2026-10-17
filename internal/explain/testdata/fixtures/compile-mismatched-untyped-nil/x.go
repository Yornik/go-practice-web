package fixture

type Point struct{ X, Y int }

func IsSet(p Point) bool {
	return p != nil
}
