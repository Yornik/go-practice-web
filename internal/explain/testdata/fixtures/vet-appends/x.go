package fixture

func Keep(lines []string, keep func(string) bool) []string {
	var out []string
	for _, l := range lines {
		if keep(l) {
			out = append(out)
		}
	}
	return out
}
