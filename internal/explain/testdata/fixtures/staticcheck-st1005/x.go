package fixture

import "errors"

func Parse(s string) error {
	if s == "" {
		return errors.New("Empty input")
	}
	return nil
}
