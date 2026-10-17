package fixture

type Tally struct{ counts map[string]int }

func (t *Tally) Add(word string) {
	t.counts[word]++
}
