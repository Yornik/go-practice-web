package fixture

import (
	"fmt"
	"io"
)

type Tally struct{ N int }

func (t *Tally) Write(p []byte) (int, error) {
	t.N += len(p)
	return len(p), nil
}

func Size(v any) int {
	var t Tally
	var w io.Writer = t
	fmt.Fprint(w, v)
	return t.N
}
