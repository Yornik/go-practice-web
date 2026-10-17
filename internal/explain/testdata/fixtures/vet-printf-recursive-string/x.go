package fixture

import "fmt"

type Celsius float64

func (c Celsius) String() string {
	return fmt.Sprintf("%v°C", c)
}
