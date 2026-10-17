package fixture

import (
	"log"
	"os"
	"strings"
)

// The test binary initialises the package before the first test, and the
// fixture has no hosts.txt: no test ever starts.
var hosts = readHosts("hosts.txt")

func readHosts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	return strings.Fields(string(data))
}

// Hosts returns the known hosts.
func Hosts() []string {
	return hosts
}
