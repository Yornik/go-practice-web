//go:build !linux

package runner

import "os/exec"

// start starts cmd unconfined: Landlock is Linux only, and other systems
// serve local development.
func (sb *sandbox) start(cmd *exec.Cmd, _ string) error { return cmd.Start() }

// describe says that nothing is enforced.
func (sb *sandbox) describe() (string, bool) {
	return "none: the sandbox needs Linux, so runs can read each other's files and signal the runner", false
}

// abi returns 0: nothing confines runs.
func (sb *sandbox) abi() int { return 0 }
