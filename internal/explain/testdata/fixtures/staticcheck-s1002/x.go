package fixture

func Label(healthy bool) string {
	if healthy == true {
		return "up"
	}
	return "down"
}
