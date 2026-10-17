package runner

import (
	"errors"
	"os/exec"

	"golang.org/x/sys/unix"
)

// waitAndKillGroup waits for the group leader to exit, kills what is left of
// its group, and only then reaps the leader. While the leader is an unreaped
// zombie its PID, and so its process group ID, cannot be reused, so the kill
// cannot hit an unrelated process group.
func waitAndKillGroup(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	// Ignore the error: the group may be empty already, and Wait below
	// reports a leader that could not be waited for.
	_ = killGroup(pid)
	return cmd.Wait()
}
