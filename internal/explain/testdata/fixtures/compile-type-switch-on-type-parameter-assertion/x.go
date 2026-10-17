package fixture

func IsText[T any](v T) bool {
	_, ok := v.(string)
	return ok
}
