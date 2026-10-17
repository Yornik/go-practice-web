package fixture

import (
	"fmt"
	"net"
)

func Connect(host string, port int) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	return net.Dial("tcp", addr)
}
