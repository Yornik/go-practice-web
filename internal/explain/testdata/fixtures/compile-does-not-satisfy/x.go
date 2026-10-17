package fixture

type Ring[K comparable, V any] struct {
	index map[K]int
	items []V
}

func Shards() int {
	var r Ring[[]byte, string]
	return len(r.items)
}
