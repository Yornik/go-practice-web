package fixture

func Name(code int) string {
	switch code
	{
	case 200:
		return "ok"
	}
	return "other"
}
