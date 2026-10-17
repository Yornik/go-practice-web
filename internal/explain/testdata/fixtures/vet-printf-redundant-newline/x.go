package fixture

import (
	"fmt"
	"io"
)

func Header(w io.Writer) {
	fmt.Fprintln(w, "status code count\n")
}
