package fixture

func Success(code int) bool {
	if code != 200 || code != 204 {
		return false
	}
	return true
}
