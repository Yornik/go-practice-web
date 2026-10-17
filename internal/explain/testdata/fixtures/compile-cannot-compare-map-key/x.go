package fixture

type TagSet struct {
	Names []string
}

func CountBy(all []TagSet) map[TagSet]int {
	counts := make(map[TagSet]int)
	for _, l := range all {
		counts[l]++
	}
	return counts
}
