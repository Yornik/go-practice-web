package fixture

import "fmt"

func Report(name string, count int) string {
	return fmt.Sprintf("%d requests from %s", name, count)
}
