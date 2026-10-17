package fixture

func FirstNegative(xs []int) int {
	found := -1
	for i, x := range xs {
		func() {
			if x < 0 {
				found = i
				break
			}
		}()
	}
	return found
}
