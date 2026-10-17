package fixture

const limit int = 1 << 62

func Max() int {
	return limit * 4
}
