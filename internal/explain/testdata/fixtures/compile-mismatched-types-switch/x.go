package fixture

func Grade(score int, limit float64) string {
	switch score {
	case limit:
		return "limit"
	}
	return "other"
}
