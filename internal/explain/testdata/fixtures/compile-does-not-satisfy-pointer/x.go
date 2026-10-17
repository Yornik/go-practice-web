package fixture

import (
	"fmt"
	"strings"
)

func JoinAll[T fmt.Stringer](xs []T) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = x.String()
	}
	return strings.Join(parts, ",")
}

type ID int

func (i *ID) String() string { return fmt.Sprint(int(*i)) }

func IDs(ids []ID) string {
	return JoinAll(ids)
}
