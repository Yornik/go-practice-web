package fixture

func Valid(size uint32) bool {
	if size < 0 {
		return false
	}
	return size <= 1<<20
}
