package fixture

type Status int

func Describe(s Status) string {
	switch s.(type) {
	case int:
		return "number"
	}
	return "other"
}
