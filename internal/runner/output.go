package runner

import (
	"bytes"
	"errors"
	"sync"
)

// errOutputCap is the cause of a run that wrote more than its output budget.
var errOutputCap = errors.New("output cap reached")

// outputBudget counts the bytes a run writes on stdout and stderr together.
// It is shared by the writers of 1 run.
type outputBudget struct {
	mu       sync.Mutex
	left     int
	exceeded bool
	onExceed func() // called once, when the budget runs out
}

func newOutputBudget(limit int, onExceed func()) *outputBudget {
	return &outputBudget{left: limit, onExceed: onExceed}
}

// take reserves n bytes. It returns false, and from then on always false,
// once the total goes over the limit.
func (b *outputBudget) take(n int) bool {
	b.mu.Lock()
	if b.exceeded {
		b.mu.Unlock()
		return false
	}
	b.left -= n
	if b.left >= 0 {
		b.mu.Unlock()
		return true
	}
	b.exceeded = true
	b.mu.Unlock()
	if b.onExceed != nil {
		b.onExceed()
	}
	return false
}

// Exceeded reports whether the run went over its budget.
func (b *outputBudget) Exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}

// lineWriter is an io.Writer that charges every write to a budget and calls
// fn once per complete line, newline included. fn must not keep the slice.
// Lines longer than maxLine are passed on in pieces so that a single huge
// line cannot grow the buffer without bound. A lineWriter is used by 1
// goroutine at a time, as os/exec does with Cmd.Stdout and Cmd.Stderr.
type lineWriter struct {
	budget  *outputBudget
	maxLine int
	fn      func(line []byte)
	buf     []byte
}

func newLineWriter(budget *outputBudget, maxLine int, fn func(line []byte)) *lineWriter {
	return &lineWriter{budget: budget, maxLine: maxLine, fn: fn}
}

// Write implements io.Writer. Over the budget it returns errOutputCap, which
// makes os/exec stop copying; the budget's onExceed has the process killed.
func (w *lineWriter) Write(p []byte) (int, error) {
	if !w.budget.take(len(p)) {
		return 0, errOutputCap
	}
	w.buf = append(w.buf, p...)
	start := 0
	for {
		rest := w.buf[start:]
		n := bytes.IndexByte(rest, '\n') + 1
		if n == 0 || n > w.maxLine {
			if len(rest) < w.maxLine {
				break
			}
			n = w.maxLine
		}
		w.fn(rest[:n])
		start += n
	}
	w.buf = w.buf[:copy(w.buf, w.buf[start:])]
	return len(p), nil
}

// Flush passes on a last line that has no newline. Call it after the
// process exited.
func (w *lineWriter) Flush() {
	if len(w.buf) > 0 && !w.budget.Exceeded() {
		w.fn(w.buf)
	}
	w.buf = nil
}
