package fixture

import (
	"slices"
	"testing"
)

func TestWords(t *testing.T) {
	got := Words("Hello World")
	want := []string{"hello", "world"}
	if !slices.Equal(got, want) {
		t.Errorf("Words(%q)\n got  %q\n want %q", "Hello World", got, want)
	}
}
