package fixture

import "fmt"

type Status int

func (s *Status) Error() string {
	return "status " + fmt.Sprint(s)
}
