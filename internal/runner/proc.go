package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds how long Wait waits for the output pipes after the
// process group is gone. Only a process that left the group can still hold
// them, and the run must not hang on it.
const waitDelay = 2 * time.Second

// command describes one child process.
type command struct {
	path   string
	args   []string
	dir    string
	env    []string
	stdout io.Writer
	stderr io.Writer
	// sandbox confines the process and everything it starts; workspace is
	// the directory it may write to besides the shared caches. A nil
	// sandbox confines nothing.
	sandbox   *sandbox
	workspace string
	// onStrays, if set, learns how many processes the sweep after the
	// command killed: processes that tried to outlive their run.
	onStrays func(killed int)
}

// run starts cmd in its own session, confined by c.sandbox, and waits for
// it. When ctx is done it kills the session's process group with SIGKILL,
// so the test binary and anything it started die with the go command. It
// also kills the group after a normal exit, and then sweeps: a process that
// left the group or the session is killed too (see tracker.sweep).
//
// A session of its own, not just a process group, keeps the run's
// processes out of the runner's process group: setpgid works only within a
// session.
//
// The exit code is -1 when the process was killed by a signal. The error is
// nil for any exit code; it reports only that the process could not run.
func run(ctx context.Context, c command) (exitCode int, err error) {
	cmd := exec.CommandContext(ctx, c.path, c.args...)
	cmd.Dir = c.dir
	cmd.Env = c.env
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	cmd.WaitDelay = waitDelay

	if err := procs.start(cmd, func() error { return c.sandbox.start(cmd, c.workspace) }); err != nil {
		return -1, fmt.Errorf("start %s: %w", c.path, err)
	}
	err = waitAndKillGroup(cmd)
	if n := procs.finished(cmd.Process.Pid); n > 0 && c.onStrays != nil {
		c.onStrays(n)
	}
	if cmd.ProcessState == nil {
		return -1, fmt.Errorf("wait %s: %w", c.path, err)
	}
	// Past this point err is a non-zero exit, a writer that gave up (the
	// output cap) or a pipe held open by a process outside the group. None
	// of them means the process did not run: the caller knows its writers,
	// and the exit code is still the truth.
	return cmd.ProcessState.ExitCode(), nil
}

// killGroup sends SIGKILL to the process group led by pid. A group that is
// already gone is not an error.
func killGroup(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group %d: %w", pid, err)
	}
	return nil
}
