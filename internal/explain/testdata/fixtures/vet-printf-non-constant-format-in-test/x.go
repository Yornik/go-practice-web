package fixture

import "fmt"

func Check(line string) error {
	if line == "" {
		msg := "empty line in " + line
		return fmt.Errorf(msg)
	}
	return nil
}
