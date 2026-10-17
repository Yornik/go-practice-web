package fixture

import "encoding/binary"

// Payload returns the payload of a record: an 8-byte big-endian length,
// then that many bytes.
func Payload(rec []byte) []byte {
	n := binary.BigEndian.Uint64(rec)
	buf := make([]byte, n)
	copy(buf, rec[8:])
	return buf
}
