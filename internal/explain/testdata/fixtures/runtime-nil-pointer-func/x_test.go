package fixture

import "testing"

func TestChange(t *testing.T) {
	w := &Watcher{}
	w.Change("a")
}
