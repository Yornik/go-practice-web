package fixture

type Point struct{ X, Y int }

func Origin() *Point {
	return *Point{X: 0, Y: 0}
}
