package fixture

import "io"

type Tally struct{ N int }

func (t *Tally) Write(p []byte) int {
	t.N += len(p)
	return len(p)
}

func Sink() io.Writer {
	return &Tally{}
}
