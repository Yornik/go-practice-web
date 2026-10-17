package fixture

// Prefix returns the first n bytes of s.
func Prefix(s string, n int) string {
	return s[:n]
}
