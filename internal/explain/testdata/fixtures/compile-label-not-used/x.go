package fixture

func Find(grid [][]int, want int) (int, int) {
	row, col := -1, -1
search:
	for i, line := range grid {
		for j, v := range line {
			if v == want {
				row, col = i, j
				break
			}
		}
	}
	return row, col
}
