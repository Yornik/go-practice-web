package fixture

import "testing"

func TestLookup(t *testing.T) {
	r := &Registry{addrs: map[string]string{"db": "10.0.0.1"}}
	if got := r.Lookup("db"); got != "10.0.0.1" {
		t.Errorf("Lookup(db) = %q, want 10.0.0.1", got)
	}
}
