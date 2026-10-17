package fixture

type Point struct{ X, Y int }

func Move(p *Point, dx int) { p.X += dx }

func Right(p Point) Point {
	Move(p, 1)
	return p
}
