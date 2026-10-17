package fixture

const target = 99 / 100

func Budget(total float64) float64 {
	return total * (1 - target)
}
