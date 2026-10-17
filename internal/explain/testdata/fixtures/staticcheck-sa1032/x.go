package fixture

import (
	"errors"
	"io"
)

func Done(err error) bool {
	return errors.Is(io.EOF, err)
}
