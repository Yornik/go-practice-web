package fixture

import (
	"testing"
	"unicode/utf8"
)

func FuzzReverse(f *testing.F) {
	f.Add("abc")
	f.Add("héllo")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		if got := Reverse(s); !utf8.ValidString(got) {
			t.Errorf("Reverse(%q) = %q, want valid UTF-8", s, got)
		}
	})
}
