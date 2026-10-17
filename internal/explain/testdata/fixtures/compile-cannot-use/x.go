package fixture

func StatusText(code int) string {
	if code == 200 {
		return "ok"
	}
	return code
}
