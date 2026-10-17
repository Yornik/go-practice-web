package fixture

import "fmt"

type Code int

func (c Code) string() string { return fmt.Sprint(int(c)) }

func Show(c Code) fmt.Stringer {
	return c
}
