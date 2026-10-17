package fixture

// Watcher reports changes to Notify, if it is set.
type Watcher struct {
	Notify func(event string)
	seen   map[string]bool
}

// Change records a change of name.
func (w *Watcher) Change(name string) {
	w.Notify("changed " + name)
}
