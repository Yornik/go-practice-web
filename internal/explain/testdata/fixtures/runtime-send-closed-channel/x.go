package fixture

// Queue buffers messages until it is closed.
type Queue struct {
	ch chan string
}

// NewQueue returns a Queue for up to 10 messages.
func NewQueue() *Queue {
	return &Queue{ch: make(chan string, 10)}
}

// Push adds s to the queue.
func (q *Queue) Push(s string) {
	q.ch <- s
}

// Close closes the queue.
func (q *Queue) Close() {
	close(q.ch)
}
