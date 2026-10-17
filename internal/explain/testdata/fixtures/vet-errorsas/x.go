package fixture

import (
	"errors"
	"io/fs"
)

func MissingPath(err error) string {
	var pe *fs.PathError
	if errors.As(err, pe) {
		return pe.Path
	}
	return ""
}
