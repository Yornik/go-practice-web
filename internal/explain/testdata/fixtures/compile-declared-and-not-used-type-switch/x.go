package fixture

func Kind(x any) string {
	switch v := x.(type) {
	case int:
		return "int"
	}
	return "other"
}
