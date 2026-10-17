package fixture

import "testing"

func TestSplit(t *testing.T) {
	for _, addr := range []string{"db:5432", "db"} {
		t.Run(addr, func(t *testing.T) {
			if host, _ := Split(addr); host != "db" {
				t.Errorf("Split(%q) host = %q, want db", addr, host)
			}
		})
	}
}
