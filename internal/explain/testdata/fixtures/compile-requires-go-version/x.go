package fixture

type Ring struct {
	items []any
}

func (r *Ring) First[T any]() (T, bool) {
	var zero T
	if len(r.items) == 0 {
		return zero, false
	}
	v, ok := r.items[0].(T)
	return v, ok
}
