package fixture

import "iter"

func Collect(seq iter.Seq[string]) []string {
	var out []string
	for _, s := range seq {
		out = append(out, s)
	}
	return out
}
