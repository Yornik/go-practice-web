package fixture

type Point struct{ X, Y int }

func Start() Point {
	return Point{1}
}
