package fixture

import (
	"fmt"
	"os"
	"strconv"
)

// Port parses a port number; 0 means there is nothing to listen on.
func Port(s string) int {
	n, err := strconv.Atoi(s)
	if err == nil && n == 0 {
		fmt.Println("port 0: nothing to do")
		os.Exit(0)
	}
	return n
}
