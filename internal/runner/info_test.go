package runner

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestInfo(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)

	status, body := env.get(t, runnerapi.PathInfo, "Bearer "+testToken)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d %s", runnerapi.PathInfo, status, body)
	}
	var info runnerapi.Info
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	// The real toolchain of this machine: the race detector works here, or
	// the integration tests that pass with -race could not pass.
	if !info.RaceAvailable || info.RaceError != "" {
		t.Errorf("info = %+v, want the race detector available", info)
	}
	if !strings.HasPrefix(info.GoVersion, "go1.") || strings.Contains(info.GoVersion, " ") {
		t.Errorf("goVersion = %q, want a release such as go1.27.2", info.GoVersion)
	}
	if info.Staticcheck != (env.srv.opts.Staticcheck != "") || info.Sandbox == "" {
		t.Errorf("info = %+v", info)
	}
	if (info.LandlockABI > 0) != strings.HasPrefix(info.Sandbox, "landlock") {
		t.Errorf("landlockABI %d does not match sandbox %q", info.LandlockABI, info.Sandbox)
	}

	for _, auth := range []string{"", "Bearer wrong"} {
		if status, body := env.get(t, runnerapi.PathInfo, auth); status != http.StatusUnauthorized || len(body) != 0 {
			t.Errorf("GET %s with %q = %d %q, want 401 without a body", runnerapi.PathInfo, auth, status, body)
		}
	}
	if status, body := env.get(t, "/readyz", ""); status != http.StatusOK || string(body) != "ok\n" {
		t.Errorf("/readyz = %d %q, want 200 ok", status, body)
	}
	// The probe's workspace is gone.
	assertWorkDirEmpty(t, env.workDir)
}

// fakeGo writes a go command that fails every invocation with -race the
// way ThreadSanitizer fails on a node whose seccomp profile blocks
// personality(ADDR_NO_RANDOMIZE), and runs the real go for everything
// else. The runner under test is the production code; only the toolchain
// it is given is broken.
func fakeGo(t *testing.T, realGo string) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "-race" ]; then
		echo "==12==FATAL: ThreadSanitizer: unexpected memory mapping 0x55d1c0400000-0x55d1c0623000" >&2
		echo "exit status 66" >&2
		exit 1
	fi
done
exec "` + realGo + `" "$@"
`
	path := filepath.Join(dir, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// syncBuffer is a bytes.Buffer for a logger that several goroutines use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRaceUnavailable(t *testing.T) {
	slow(t)
	t.Parallel()
	tc, err := toolchain()
	if err != nil {
		t.Fatal(err)
	}
	var logs syncBuffer
	env := newTestEnv(t, func(o *Options) {
		o.Go = fakeGo(t, tc.goPath)
		o.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	})

	status, body := env.get(t, runnerapi.PathInfo, "Bearer "+testToken)
	var info runnerapi.Info
	if err := json.Unmarshal(body, &info); status != http.StatusOK || err != nil {
		t.Fatalf("GET %s = %d %s", runnerapi.PathInfo, status, body)
	}
	if info.RaceAvailable || !strings.Contains(info.RaceError, "FATAL: ThreadSanitizer: unexpected memory mapping") {
		t.Errorf("info = %+v, want the race detector unavailable with the ThreadSanitizer line", info)
	}
	if n := strings.Count(logs.String(), "race detector not available"); n != 1 ||
		!strings.Contains(logs.String(), `"level":"WARN","msg":"race detector not available`) {
		t.Errorf("want 1 WARN line about the race detector, got %d in:\n%s", n, logs.String())
	}
	if status, body := env.get(t, "/readyz", ""); status != http.StatusOK ||
		!strings.HasPrefix(string(body), "ok\nrace detector not available: ") {
		t.Errorf("/readyz = %d %q, want ready with the race detector named", status, body)
	}

	// The fake go fails anything with -race, so a pass shows that the
	// runner left it out.
	events := env.runTests(t, runnerapi.TestRequest{Exercise: "01-logfmt", Files: solutionFiles(t, "01-logfmt"), Race: true})
	done := doneEvent(t, events)
	if done.ExitCode != 0 || done.Passed == 0 || !done.RaceUnavailable {
		t.Errorf("done = %+v, want a pass with raceUnavailable", done)
	}
	// A run that did not ask for -race lost nothing.
	events = env.runTests(t, runnerapi.TestRequest{Exercise: "01-logfmt", Files: solutionFiles(t, "01-logfmt"), Run: "TestLevel"})
	if done := doneEvent(t, events); done.ExitCode != 0 || done.RaceUnavailable {
		t.Errorf("done without race = %+v, want a pass without raceUnavailable", done)
	}
	if n := strings.Count(logs.String(), "race detector not available"); n != 1 {
		t.Errorf("the warning was logged %d times, want once", n)
	}
	assertWorkDirEmpty(t, env.workDir)
}

func TestRaceFailure(t *testing.T) {
	tests := []struct {
		name string
		out  string
		code int
		want string
	}{
		{
			name: "ThreadSanitizer",
			out:  "# raceprobe\n==7==FATAL: ThreadSanitizer: failed to personality()\nexit status 66\n",
			code: 1,
			want: "go run -race exited with code 1: ==7==FATAL: ThreadSanitizer: failed to personality()",
		},
		{
			name: "no cgo",
			out:  "go: -race requires cgo; enable cgo by setting CGO_ENABLED=1\n",
			code: 1,
			want: "go run -race exited with code 1: go: -race requires cgo; enable cgo by setting CGO_ENABLED=1",
		},
		{name: "first line", out: "\nsomething odd\nmore\n", code: 2, want: "go run -race exited with code 2: something odd"},
		{name: "silent", out: "", code: -1, want: "go run -race exited with code -1: no output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := raceFailure(tt.out, tt.code); got != tt.want {
				t.Errorf("raceFailure = %q, want %q", got, tt.want)
			}
		})
	}
	if got := raceFailure(strings.Repeat("x", 1000), 1); len(got) > 400 {
		t.Errorf("a long line is not cut: %d bytes", len(got))
	}
}

func TestGoRelease(t *testing.T) {
	for in, want := range map[string]string{
		"go version go1.27.2 linux/amd64":           "go1.27.2",
		"go version devel go1.28-abc linux/amd64":   "devel go1.28-abc",
		"something else":                            "something else",
		"go version go1.26.0 X:nocoverageredesign ": "go1.26.0",
	} {
		if got := goRelease(in); got != want {
			t.Errorf("goRelease(%q) = %q, want %q", in, got, want)
		}
	}
}
