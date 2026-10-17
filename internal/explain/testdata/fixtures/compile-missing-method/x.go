package fixture

import "io"

type Blank struct{}

func Source() io.Reader {
	return Blank{}
}
