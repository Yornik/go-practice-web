package fixture

import "strings"

func Greeting(name string) string {
	var msg string
	msg = "hello, " + strings.TrimSpace(name)
	return msg
}
