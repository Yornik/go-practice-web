package fixture

import "testing"

// TestWatch passes, but leaves the goroutine of Watch behind: nothing
// closes in. The dump of the timeout lists it before the test that hangs.
func TestWatch(t *testing.T) {
	in := make(chan int)
	got := make(chan int)
	go Watch(in, func(v int) { got <- v })
	in <- 1
	if v := <-got; v != 1 {
		t.Errorf("reported %d, want 1", v)
	}
}

func TestSum(t *testing.T) {
	tests := []struct {
		name string
		xs   []int
		want int
	}{
		{"four numbers", []int{1, 2, 3, 4}, 10},
	}
	for _, tc := range tests {
		// While the subtest hangs, TestSum waits on this line.
		t.Run(tc.name, func(t *testing.T) {
			if got := Sum(tc.xs); got != tc.want {
				t.Errorf("Sum = %d, want %d", got, tc.want)
			}
		})
	}
}
