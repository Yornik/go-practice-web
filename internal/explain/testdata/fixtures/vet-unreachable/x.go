package fixture

import (
	"log"
	"strconv"
)

func Parse(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
		log.Printf("bad number %q", s)
	}
	return n, nil
}
