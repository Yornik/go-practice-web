package fixture

import "strings"

func Version(tag string) string {
	if strings.HasPrefix(tag, "v") {
		tag = tag[1:]
	}
	return tag
}
