package fixture

import "sync"

type Stats struct {
	mu    sync.Mutex
	count int
}

func (s *Stats) Add() {
	s.mu.Lock()
	s.mu.Unlock()
	s.count++
}
