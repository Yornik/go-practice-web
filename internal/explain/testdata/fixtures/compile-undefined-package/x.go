package fixture

// Pair formats a key and a value.
func Pair(k, v string) string {
	return fmt.Sprintf("%s=%q", k, v)
}
