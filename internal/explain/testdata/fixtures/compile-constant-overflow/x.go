package fixture

func Header() []byte {
	var flags byte = 256
	return []byte{flags}
}
