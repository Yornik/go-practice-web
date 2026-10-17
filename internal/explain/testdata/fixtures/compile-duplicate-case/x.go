package fixture

func Severity(level string) int {
	switch level {
	case "warn", "warning":
		return 2
	case "error":
		return 3
	case "warn":
		return 1
	}
	return 0
}
