package fixture

type Index[K comparable, V any] struct{ m map[K]V }

func New() *Index[string, int] {
	c := Index[string, int]
	return &c
}
