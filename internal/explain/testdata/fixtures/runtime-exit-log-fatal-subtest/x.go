package fixture

import (
	"log"
	"strings"
)

// Split splits a host:port address.
func Split(addr string) (host, port string) {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		fail("no port in %q", addr)
	}
	return host, port
}

func fail(format string, args ...any) {
	log.Fatalf(format, args...)
}
