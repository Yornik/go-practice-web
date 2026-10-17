package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// harden makes the runner process non-dumpable. Learner code runs as the
// same user; without this it could read the runner's memory through
// /proc/<pid>/mem or ptrace and see other learners' code in flight. The
// children are not affected: execve resets the flag.
func harden() error {
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_DUMPABLE): %w", err)
	}
	return nil
}
