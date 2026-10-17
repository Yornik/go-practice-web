package fixture

func Length(b []byte) uint16 {
	return uint16(b[0]<<8 | b[1])
}
