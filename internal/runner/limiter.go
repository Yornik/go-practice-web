package runner

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// ErrQueueFull is returned by Limiter.Join when every slot is busy and the
// queue holds its maximum number of waiting requests.
var ErrQueueFull = errors.New("runner: queue full")

// Limiter bounds the number of concurrent go invocations and keeps a FIFO
// queue of bounded length for the requests that wait for a slot.
//
// A plain semaphore would not do: the queue must be bounded (so that the
// runner answers 429 instead of piling up work) and every waiter must know
// its place in line (so that the learner sees it).
type Limiter struct {
	mu       sync.Mutex
	slots    int
	maxQueue int
	running  int
	queue    []*Ticket

	// onChange, when set, receives the running and queued counts after every
	// change, under mu, so that the values arrive in order.
	onChange func(running, queued int)
}

// NewLimiter returns a Limiter with the given number of slots and queue
// length. slots must be at least 1 and maxQueue at least 0.
func NewLimiter(slots, maxQueue int) *Limiter {
	return &Limiter{slots: max(slots, 1), maxQueue: max(maxQueue, 0)}
}

// Ticket is a request's place at the Limiter: either a slot or a place in
// the queue. Release it exactly when the work is done or abandoned.
type Ticket struct {
	l       *Limiter
	granted chan struct{} // closed when the ticket gets a slot
	moved   chan struct{} // signalled when the queue ahead changed
	state   ticketState   // guarded by l.mu
}

type ticketState int

const (
	ticketQueued ticketState = iota
	ticketRunning
	ticketReleased
)

// Join takes a slot if one is free and nobody is waiting, and otherwise
// joins the queue. It returns ErrQueueFull when the queue is full.
func (l *Limiter) Join() (*Ticket, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	t := &Ticket{l: l, granted: make(chan struct{}), moved: make(chan struct{}, 1)}
	if l.running < l.slots && len(l.queue) == 0 {
		l.running++
		t.state = ticketRunning
		close(t.granted)
		l.changedLocked()
		return t, nil
	}
	if len(l.queue) >= l.maxQueue {
		return nil, ErrQueueFull
	}
	l.queue = append(l.queue, t)
	l.changedLocked()
	return t, nil
}

// Wait blocks until the ticket has a slot or ctx is done. While the ticket
// waits, Wait calls onPosition (if not nil) from the calling goroutine with
// the number of requests ahead of it, running ones included, first right
// away and then every time that number changes. Running requests count
// because a queued request always has at least 1 of them ahead, so the
// number is never 0 and a client can show it as a place in line.
func (t *Ticket) Wait(ctx context.Context, onPosition func(position int)) error {
	last := -1
	for {
		select {
		case <-t.granted:
			return nil
		default:
		}
		if onPosition != nil {
			if pos := t.position(); pos > 0 && pos != last {
				last = pos
				onPosition(pos)
			}
		}
		select {
		case <-t.granted:
			return nil
		case <-t.moved:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

// position returns the number of requests ahead of t, or 0 when t is no
// longer queued.
func (t *Ticket) position() int {
	l := t.l
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, q := range l.queue {
		if q == t {
			return l.running + i
		}
	}
	return 0
}

// Release gives the slot back or leaves the queue. It is safe to call more
// than once and from any goroutine.
func (t *Ticket) Release() {
	l := t.l
	l.mu.Lock()
	defer l.mu.Unlock()

	switch t.state {
	case ticketReleased:
		return
	case ticketQueued:
		for i, q := range l.queue {
			if q == t {
				l.queue = slices.Delete(l.queue, i, i+1)
				l.notifyFrom(i)
				break
			}
		}
	case ticketRunning:
		l.running--
		if len(l.queue) > 0 {
			next := l.queue[0]
			l.queue = slices.Delete(l.queue, 0, 1)
			next.state = ticketRunning
			l.running++
			close(next.granted)
			l.notifyFrom(0)
		}
	}
	t.state = ticketReleased
	l.changedLocked()
}

// notifyFrom tells the waiters from index i on that their position changed.
// The send never blocks: a pending signal already covers the new change.
func (l *Limiter) notifyFrom(i int) {
	for _, q := range l.queue[i:] {
		select {
		case q.moved <- struct{}{}:
		default:
		}
	}
}

func (l *Limiter) changedLocked() {
	if l.onChange != nil {
		l.onChange(l.running, len(l.queue))
	}
}

// Stats returns the number of running and queued requests.
func (l *Limiter) Stats() (running, queued int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running, len(l.queue)
}
