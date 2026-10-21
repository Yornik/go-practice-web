package runner

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMain warms the build cache before the integration tests run in
// parallel. Each go process compiles what the cache lacks on its own, so on
// a cold cache (a fresh CI machine) 8 parallel runs would each compile the
// standard library with -race, and the timeout test could time out in the
// build instead of in the test binary.
func TestMain(m *testing.M) {
	// First, while the main goroutine still runs on the main thread.
	probeMainThread()
	flag.Parse()
	if !testing.Short() {
		if err := warmCache(); err != nil {
			fmt.Fprintln(os.Stderr, "warm the build cache:", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// warmCache builds what the integration tests need, in the course checkout
// and with the environment the runner gives its children, so that the cache
// keys match. It writes nothing into the checkout.
func warmCache() error {
	tc, err := toolchain()
	if err != nil {
		return err
	}
	start := time.Now()
	env := append(os.Environ(),
		"GOCACHE="+tc.goCache, "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local", "CGO_ENABLED=1")
	for _, args := range [][]string{
		{"vet", "./exercises/26-logfmt/", "./exercises/33-ratelimit/", "./exercises/37-workerpool/"},
		{"test", "-count=1", "-run=^$", "./exercises/26-logfmt/"},
		{"test", "-count=1", "-run=^$", "./exercises/43-certcheck/"},
		{"test", "-race", "-count=1", "-run=^$", "./exercises/01-loglevels/", "./exercises/26-logfmt/", "./exercises/37-workerpool/", "./exercises/40-healthcheck/"},
	} {
		cmd := exec.Command(tc.goPath, args...)
		cmd.Dir = courseDir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go %v: %w\n%s", args, err, out)
		}
	}
	if d := time.Since(start); d > 10*time.Second {
		fmt.Fprintf(os.Stderr, "warmed the build cache in %s\n", d.Round(time.Second))
	}
	return nil
}
