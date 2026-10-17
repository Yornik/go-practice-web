package fixture

import "sync"

type Worker struct {
	mu   sync.Mutex
	jobs chan func()
}

func (w *Worker) Run() {
	for {
		job := <-w.jobs
		w.mu.Lock()
		defer w.mu.Unlock()
		job()
	}
}
