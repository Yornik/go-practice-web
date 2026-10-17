package fixture

func Capitalize(s string) string {
	if s == "" {
		return s
	}
	s[0] = s[0] - 'a' + 'A'
	return s
}
