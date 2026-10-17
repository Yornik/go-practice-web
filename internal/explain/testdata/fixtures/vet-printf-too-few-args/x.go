package fixture

import "fmt"

func Progress(name string, done, total int) string {
	return fmt.Sprintf("%s: %d of %d done", name, done)
}
