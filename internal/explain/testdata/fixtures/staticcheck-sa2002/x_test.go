package fixture

import "testing"

func TestFetch(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := Fetch(); err != nil {
			t.Fatal(err)
		}
	}()
	<-done
}
