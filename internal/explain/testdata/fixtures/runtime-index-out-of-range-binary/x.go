package fixture

import "encoding/binary"

// Length reads the 4-byte big-endian length at the start of a record.
func Length(rec []byte) uint32 {
	return binary.BigEndian.Uint32(rec)
}
