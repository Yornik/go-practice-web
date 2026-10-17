//go:build unix && !linux

package runner

import "os/exec"

// waitAndKillGroup reaps the leader, then kills what is left of its group.
// Without waitid(WNOWAIT) the leader's PID could in theory be reused between
// the two steps; the runner image runs on Linux, so this path only serves
// local development.
func waitAndKillGroup(cmd *exec.Cmd) error {
	err := cmd.Wait()
	_ = killGroup(cmd.Process.Pid)
	return err
}
