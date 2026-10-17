package fixture

type Ring[T any] struct {
	items []T
}

func (r *Ring) Len() int {
	return len(r.items)
}
