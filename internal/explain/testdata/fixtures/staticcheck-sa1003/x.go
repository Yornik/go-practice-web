package fixture

import (
	"encoding/binary"
	"io"
)

func WriteLength(w io.Writer, n int) error {
	return binary.Write(w, binary.BigEndian, n)
}
