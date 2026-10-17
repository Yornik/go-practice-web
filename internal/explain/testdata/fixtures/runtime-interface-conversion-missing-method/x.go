package fixture

import "io"

// Drain reads r to the end and closes it.
func Drain(r io.Reader) error {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}
	return r.(io.Closer).Close()
}
