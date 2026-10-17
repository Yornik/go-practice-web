package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitTimeout bounds every blocking wait in the limiter tests.
const waitTimeout = 5 * time.Second

func mustJoin(t *testing.T, l *Limiter) *Ticket {
	t.Helper()
	ticket, err := l.Join()
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	return ticket
}

func isGranted(t *Ticket) bool {
	select {
	case <-t.granted:
		return true
	default:
		return false
	}
}

func TestLimiterGrantsSlotsThenQueuesThenRefuses(t *testing.T) {
	tests := []struct {
		name          string
		slots, queue  int
		joins         int
		wantGranted   int
		wantQueued    int
		wantQueueFull int
	}{
		{name: "within slots", slots: 2, queue: 1, joins: 2, wantGranted: 2},
		{name: "fills the queue", slots: 2, queue: 2, joins: 4, wantGranted: 2, wantQueued: 2},
		{name: "refuses past the queue", slots: 1, queue: 2, joins: 5, wantGranted: 1, wantQueued: 2, wantQueueFull: 2},
		{name: "no queue", slots: 1, queue: 0, joins: 3, wantGranted: 1, wantQueueFull: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLimiter(tt.slots, tt.queue)
			var granted, queued, full int
			for range tt.joins {
				ticket, err := l.Join()
				switch {
				case errors.Is(err, ErrQueueFull):
					full++
				case err != nil:
					t.Fatalf("Join: %v", err)
				case isGranted(ticket):
					granted++
				default:
					queued++
				}
			}
			if granted != tt.wantGranted || queued != tt.wantQueued || full != tt.wantQueueFull {
				t.Errorf("granted, queued, full = %d, %d, %d; want %d, %d, %d",
					granted, queued, full, tt.wantGranted, tt.wantQueued, tt.wantQueueFull)
			}
			if running, inQueue := l.Stats(); running != tt.wantGranted || inQueue != tt.wantQueued {
				t.Errorf("Stats = %d, %d; want %d, %d", running, inQueue, tt.wantGranted, tt.wantQueued)
			}
		})
	}
}

func TestLimiterIsFIFOAndReportsPositions(t *testing.T) {
	l := NewLimiter(1, 3)
	first := mustJoin(t, l)
	second := mustJoin(t, l)
	third := mustJoin(t, l)

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	positions := make(chan int, 10)
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- third.Wait(ctx, func(p int) { positions <- p })
	}()
	// 1 running and 1 queued request are ahead of the third.
	if got := receive(t, positions); got != 2 {
		t.Fatalf("first position = %d, want 2", got)
	}

	first.Release()
	if !isGranted(second) {
		t.Fatal("second ticket was not granted after the first was released")
	}
	if got := receive(t, positions); got != 1 {
		t.Fatalf("position after a release = %d, want 1", got)
	}

	second.Release()
	if err := receive(t, waitErr); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	third.Release()
	if running, queued := l.Stats(); running != 0 || queued != 0 {
		t.Errorf("Stats after all releases = %d, %d; want 0, 0", running, queued)
	}
}

func TestLimiterCancelledWaiterLeavesTheQueue(t *testing.T) {
	l := NewLimiter(1, 2)
	running := mustJoin(t, l)
	leaving := mustJoin(t, l)
	staying := mustJoin(t, l)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := leaving.Wait(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait on a cancelled context = %v, want context.Canceled", err)
	}
	leaving.Release()
	if got := staying.position(); got != 1 {
		t.Errorf("position after the waiter ahead left = %d, want 1", got)
	}

	// The released waiter must not take the slot; the one that stayed does.
	running.Release()
	if isGranted(leaving) {
		t.Error("a released waiter was granted a slot")
	}
	if !isGranted(staying) {
		t.Error("the remaining waiter was not granted the free slot")
	}
	staying.Release()
	staying.Release() // a second Release is a no-op
	if r, q := l.Stats(); r != 0 || q != 0 {
		t.Errorf("Stats = %d, %d; want 0, 0", r, q)
	}
}

func TestLimiterReportsChanges(t *testing.T) {
	l := NewLimiter(1, 1)
	type stats struct{ running, queued int }
	var got []stats
	l.onChange = func(running, queued int) { got = append(got, stats{running, queued}) }

	a := mustJoin(t, l)
	b := mustJoin(t, l)
	a.Release()
	b.Release()

	want := []stats{{1, 0}, {1, 1}, {1, 0}, {0, 0}}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("change %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// receive reads 1 value or fails the test after waitTimeout.
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	return receiveWithin(t, ch, waitTimeout)
}
