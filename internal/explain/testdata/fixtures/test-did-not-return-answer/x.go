package fixture

import (
	"net/http"
	"sync"
)

// Status serves the number of requests so far.
type Status struct {
	mu sync.Mutex
	n  int
}

func (s *Status) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.n++
	if r.URL.Query().Get("reset") != "" {
		return
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}
