package fixture

func Check(port int) error {
	if port <= 0 {
		return "invalid port"
	}
	return nil
}
