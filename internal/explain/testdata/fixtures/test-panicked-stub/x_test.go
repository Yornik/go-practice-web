package fixture

import (
	"testing"
	"time"
)

func TestSum(t *testing.T) {
	var got int
	within(t, "Sum([1 2])", func() { got = Sum([]int{1, 2}) })
	if got != 3 {
		t.Errorf("Sum([1 2]) = %d, want 3", got)
	}
}

// timeout is short here; the course tests wait 5s.
const timeout = 100 * time.Millisecond

// within runs f in a new goroutine and waits for it. It fails the test if
// f panics or does not return within the timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
	}
}
