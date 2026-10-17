package fixture

import "testing"

func call() (err error, panicked any) {
	defer func() { panicked = recover() }()
	return Fetch(), nil
}

func TestFetch(t *testing.T) {
	err, p := call()
	if err == nil {
		t.Errorf("Fetch() = %v (panic: %v), want an error", err, p)
	}
}
