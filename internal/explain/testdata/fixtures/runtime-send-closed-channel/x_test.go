package fixture

import "testing"

func TestPushAfterClose(t *testing.T) {
	q := NewQueue()
	q.Close()
	q.Push("late")
}
