package fixture

type Limits struct{ Retries int }

func Default() Limits {
	return Limits
}
