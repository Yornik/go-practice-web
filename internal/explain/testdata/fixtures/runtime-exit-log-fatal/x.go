package fixture

import (
	"log"
	"strconv"
)

// Port parses a port number.
func Port(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Fatal(err)
	}
	return n
}
