package fixture

import "slices"

func Remove(ids []string, i int) []string {
	slices.Delete(ids, i, i+1)
	return ids
}
