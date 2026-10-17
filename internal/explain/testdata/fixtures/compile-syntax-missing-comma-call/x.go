package fixture

import "fmt"

func Line(host string, port int) string {
	return fmt.Sprintf(
		"%s:%d",
		host,
		port
	)
}
