package fixture

import "net"

func Local(port int) net.TCPAddr {
	return net.TCPAddr{net.IPv4(127, 0, 0, 1), port, ""}
}
