package runner

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// raceProbeTimeout bounds the race probe at start. With the image's seeded
// build cache it takes about a second; a cold cache first compiles the
// runtime with -race.
const raceProbeTimeout = 2 * time.Minute

// raceProbeOK is what the probe program prints when it ran to the end.
const raceProbeOK = "race probe ok"

// raceProbeMain starts a goroutine and waits for it, so the race runtime
// sets up and tracks 2 goroutines, as in any test.
const raceProbeMain = `package main

import "fmt"

func main() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
	fmt.Println("` + raceProbeOK + `")
}
`

// maxProbeOutput bounds what the probe keeps of its output. A failure
// prints a few lines; the reason is in the first of them.
const maxProbeOutput = 64 << 10

// Probe checks the toolchain once at start and keeps the result for GET
// /v1/info, /readyz and the test runs: go version must work, and a tiny
// program built with -race must run in the sandbox of runs. A race
// detector that does not work is not an error: test runs then go without
// -race (the done event says so), and the runner logs 1 warning. Call
// Probe before serving requests.
func (s *Server) Probe(ctx context.Context) (runnerapi.Info, error) {
	version, err := s.GoVersion(ctx)
	if err != nil {
		return runnerapi.Info{}, err
	}
	sandbox, _ := s.sandbox.describe()
	info := runnerapi.Info{
		GoVersion:     goRelease(version),
		RaceAvailable: true,
		Staticcheck:   s.opts.Staticcheck != "",
		Sandbox:       sandbox,
		LandlockABI:   s.sandbox.abi(),
	}
	reason, err := s.probeRace(ctx)
	if err != nil {
		return runnerapi.Info{}, err
	}
	if reason != "" {
		info.RaceAvailable, info.RaceError = false, reason
		s.log.Warn("race detector not available: test runs go without -race", "reason", reason)
	}
	s.info.Store(&info)
	return info, nil
}

// raceAvailable reports whether test runs may use -race. Before Probe ran
// nothing is known, and go test reports a broken race detector itself.
func (s *Server) raceAvailable() bool {
	info := s.info.Load()
	return info == nil || info.RaceAvailable
}

// probeRace builds and runs raceProbeMain with go run -race in a scratch
// workspace, with the environment and the sandbox of a learner's run, so
// that it fails where a test run with -race would. It returns the reason
// when the race detector does not work, "" when it does, and an error when
// the probe could not run at all.
func (s *Server) probeRace(ctx context.Context) (reason string, err error) {
	ctx, cancel := context.WithTimeout(ctx, raceProbeTimeout)
	defer cancel()
	ws, err := newScratch(s.opts.WorkDir)
	if err != nil {
		return "", fmt.Errorf("race probe: %w", err)
	}
	defer s.closeWorkspace(ctx, ws)
	// The module lives next to HOME and GOPATH, as in a run's workspace.
	ws.modDir = filepath.Join(ws.root, "mod")
	if err := os.Mkdir(ws.modDir, 0o700); err != nil {
		return "", fmt.Errorf("race probe: %w", err)
	}
	for name, content := range map[string]string{
		"go.mod":  "module raceprobe\n\ngo 1.24\n",
		"main.go": raceProbeMain,
	} {
		if err := os.WriteFile(filepath.Join(ws.modDir, name), []byte(content), 0o600); err != nil {
			return "", fmt.Errorf("race probe: %w", err)
		}
	}

	out := &cappedBuffer{max: maxProbeOutput}
	code, err := run(ctx, command{
		path: s.opts.Go, args: []string{"run", "-race", "."}, dir: ws.modDir,
		env: s.env(ws), stdout: out, stderr: out,
		sandbox: s.sandbox, workspace: ws.root, onStrays: s.reportStrays,
	})
	if err != nil {
		return "", fmt.Errorf("race probe: %w", err)
	}
	if code == 0 && strings.Contains(out.String(), raceProbeOK) {
		return "", nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("the race probe did not finish within %s", raceProbeTimeout), nil
	}
	return raceFailure(out.String(), code), nil
}

// raceFailure picks the line of the probe's output that names the problem,
// for example "FATAL: ThreadSanitizer: unexpected memory mapping ...".
func raceFailure(out string, code int) string {
	var first, named string
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "exit status ") {
			continue
		}
		if first == "" {
			first = line
		}
		if named == "" && (strings.Contains(line, "ThreadSanitizer") || strings.Contains(line, "FATAL") ||
			strings.Contains(line, "-race")) {
			named = line
		}
	}
	line := cmp.Or(named, first, "no output")
	const maxReason = 300
	if len(line) > maxReason {
		line = line[:maxReason] + "..."
	}
	return fmt.Sprintf("go run -race exited with code %d: %s", code, line)
}

// goRelease returns "go1.27.2" for "go version go1.27.2 linux/amd64",
// "devel go1.28-abc" for a development build, or the input when it has
// another form.
func goRelease(version string) string {
	f := strings.Fields(version)
	switch {
	case len(f) >= 4 && f[0] == "go" && f[1] == "version" && f[2] == "devel":
		return "devel " + f[3]
	case len(f) >= 3 && f[0] == "go" && f[1] == "version":
		return f[2]
	}
	return version
}

// handleInfo answers GET /v1/info with what Probe found. It needs the
// token like the rest of the API, and it answers while the runner drains:
// it starts no work.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	info := s.info.Load()
	if info == nil {
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, "not_ready", "the runner has not checked its toolchain yet")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// cappedBuffer keeps the first max bytes written to it and drops the rest
// without an error, so that a chatty command is not cut short by its
// writer.
type cappedBuffer struct {
	max int
	buf bytes.Buffer
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string { return b.buf.String() }
