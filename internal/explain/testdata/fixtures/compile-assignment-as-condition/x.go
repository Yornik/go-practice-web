package fixture

func Status(code int) string {
	if code = 200 {
		return "ok"
	}
	return "error"
}
