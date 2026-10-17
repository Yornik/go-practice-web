package fixture

import "regexp"

var level = regexp.MustCompile(`level=(\w+`)

func Level(line string) string { return level.FindString(line) }
