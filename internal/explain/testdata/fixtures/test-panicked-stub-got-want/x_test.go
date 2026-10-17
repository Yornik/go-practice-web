package fixture

import (
	"fmt"
	"testing"
)

func call() (err error, panicked any) {
	defer func() { panicked = recover() }()
	return Fetch(), nil
}

func TestFetch(t *testing.T) {
	err, p := call()
	got := fmt.Sprint(err)
	if p != nil {
		got = fmt.Sprintf("Fetch panicked: %v", p)
	}
	if err == nil {
		t.Errorf("Fetch() = %s, want an error", got)
	}
}
