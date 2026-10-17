package fixture

import (
	"fmt"
	"os"
	"strconv"
)

// Port parses a port number.
func Port(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad port:", s)
		os.Exit(1)
	}
	return n
}
