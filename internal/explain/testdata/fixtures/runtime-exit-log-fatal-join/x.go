package fixture

import (
	"errors"
	"fmt"
	"log"
	"strconv"
)

// Ports parses every port and stops at the end if any of them is bad.
func Ports(ss []string) []int {
	var ports []int
	var errs []error
	for _, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			errs = append(errs, fmt.Errorf("port %q: %w", s, err))
			continue
		}
		ports = append(ports, n)
	}
	if err := errors.Join(errs...); err != nil {
		log.Fatal(err)
	}
	return ports
}
