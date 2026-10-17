package fixture

func IsServerError(code int) bool {
	if code >= 500 {
		return true
	}
	return false
}
