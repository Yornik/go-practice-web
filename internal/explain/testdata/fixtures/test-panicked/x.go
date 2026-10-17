package fixture

// Tally counts words.
type Tally struct {
	counts map[string]int
}

// Add counts 1 more w.
func (t *Tally) Add(w string) {
	t.counts[w]++
}
