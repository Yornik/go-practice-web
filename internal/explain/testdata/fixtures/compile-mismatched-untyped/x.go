package fixture

func Key(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == "=" {
			return line[:i]
		}
	}
	return line
}
