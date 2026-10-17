//go:build !linux

package runner

import "os/exec"

// enableSubreaper does nothing: child subreapers are Linux only.
func enableSubreaper() error { return nil }

// tracker does nothing outside Linux, where the runner image runs; there a
// process that leaves its session survives the run.
type tracker struct{}

var procs = &tracker{}

func (t *tracker) start(_ *exec.Cmd, startFn func() error) error { return startFn() }

func (t *tracker) finished(int) int { return 0 }

func (t *tracker) sweep() int { return 0 }
