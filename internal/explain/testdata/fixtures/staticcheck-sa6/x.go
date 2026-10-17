package fixture

import "regexp"

func Count(lines []string) int {
	n := 0
	for _, l := range lines {
		if ok, _ := regexp.MatchString(`^ERROR`, l); ok {
			n++
		}
	}
	return n
}
