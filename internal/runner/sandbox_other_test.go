//go:build !linux

package runner

import "testing"

// probeMainThread does nothing: the sandbox is Linux only.
func probeMainThread() {}

// assertNoStrays does nothing: only Linux sweeps for strays.
func assertNoStrays(*testing.T, *testEnv) {}
