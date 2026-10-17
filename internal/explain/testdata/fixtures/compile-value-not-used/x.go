package fixture

func Track(hits []string, path string) []string {
	append(hits, path)
	return hits
}
