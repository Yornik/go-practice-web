package fixture

type Ring[T any] struct {
	items []T
}

func (r *Ring[T any]) Len() int {
	return len(r.items)
}
