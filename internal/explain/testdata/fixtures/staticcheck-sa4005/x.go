package fixture

type Point struct {
	X, Y int
}

func (p Point) Move(dx, dy int) {
	p.X += dx
	p.Y += dy
}
