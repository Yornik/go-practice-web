package fixture

import "fmt"

func Describe(err error) string {
	return fmt.Sprintf("request failed: %s", err.Error)
}
