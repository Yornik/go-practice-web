package fixture

import (
	"os"
	"strings"
)

var hosts []string

// The fixture has no hosts.txt, and the binary prints nothing before it
// exits, so go test prints "exit status 1".
func init() {
	data, err := os.ReadFile("hosts.txt")
	if err != nil {
		os.Exit(1)
	}
	hosts = strings.Fields(string(data))
}

// Hosts returns the known hosts.
func Hosts() []string {
	return hosts
}
