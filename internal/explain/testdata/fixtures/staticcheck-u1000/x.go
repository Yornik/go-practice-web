package fixture

import "strings"

func Normalize(s string) string { return strings.ToLower(s) }

func trimAll(xs []string) []string {
	for i := range xs {
		xs[i] = strings.TrimSpace(xs[i])
	}
	return xs
}
