package fixture

func Name(v int) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
