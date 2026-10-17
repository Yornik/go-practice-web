package fixture

func Status(failures int) string {
	if failures {
		return "failing"
	}
	return "ok"
}
