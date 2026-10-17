package runner

// These tests run the real go toolchain on the course submodule. They are
// skipped with -short.

import (
	"cmp"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// lineOf returns the 1-based line of the first line in src that contains
// substr.
func lineOf(t *testing.T, src, substr string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, substr) {
			return i + 1
		}
	}
	t.Fatalf("%q not found", substr)
	return 0
}

// withFmtImport adds "fmt" to the import block of the 01-logfmt stub.
func withFmtImport(t *testing.T, src string) string {
	t.Helper()
	out := strings.Replace(src, "\t\"errors\"\n", "\t\"errors\"\n\t\"fmt\"\n", 1)
	if out == src {
		t.Fatal("the 01-logfmt stub no longer imports errors; adjust the test")
	}
	return out
}

func TestCheck(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)
	t.Cleanup(func() { assertWorkDirEmpty(t, env.workDir) })
	stub := stubFiles(t, "01-logfmt")["logfmt.go"]

	typeErr := stub + "\nfunc Bad() int {\n\treturn \"x\"\n}\n"
	vetErr := withFmtImport(t, stub) + "\nfunc Bad(s string) {\n\tfmt.Printf(\"%d\\n\", s)\n}\n"
	syntaxErr := stub + "\nfunc Bad( {\n}\n"
	apiBreak := strings.Replace(stub, "func Parse(line string)", "func Parse2(line string)", 1)

	unused := stub + "\nfunc unused() {}\n"
	hasStaticcheck := env.srv.opts.Staticcheck != ""

	tests := []struct {
		name     string
		exercise string            // default 01-logfmt
		files    map[string]string // default logfmt.go with content
		content  string
		want     []runnerapi.Diagnostic // checked by file, line, source, severity and code
		exact    bool                   // no other diagnostics allowed
		only     string                 // if set, the only source allowed
		needSC   bool                   // needs staticcheck
	}{
		{name: "unmodified stub", content: stub, exact: true},
		{
			// The course's test files have staticcheck findings; they are
			// not the learner's to fix and must not show up.
			name: "unmodified stub with noisy tests", exercise: "08-ratelimit",
			files: stubFiles(t, "08-ratelimit"), exact: true,
		},
		{
			name:    "type error",
			content: typeErr,
			want: []runnerapi.Diagnostic{{
				File: "logfmt.go", Line: lineOf(t, typeErr, `return "x"`), Column: 9,
				Severity: "error", Source: "compile",
			}},
		},
		{
			name:    "printf mismatch",
			content: vetErr,
			want: []runnerapi.Diagnostic{{
				File: "logfmt.go", Line: lineOf(t, vetErr, "fmt.Printf"),
				Severity: "warning", Source: "vet", Code: "printf",
			}},
		},
		{
			name:    "staticcheck finding",
			content: unused,
			want: []runnerapi.Diagnostic{{
				File: "logfmt.go", Line: lineOf(t, unused, "func unused"),
				Severity: "warning", Source: "staticcheck", Code: "U1000",
			}},
			needSC: true,
		},
		{
			name:    "syntax error",
			content: syntaxErr,
			want: []runnerapi.Diagnostic{{
				File: "logfmt.go", Line: lineOf(t, syntaxErr, "func Bad( {"),
				Severity: "error", Source: "gofmt",
			}},
			// A file that does not parse skips vet and staticcheck.
			only: runnerapi.SourceGofmt,
		},
		{
			name:    "error in a test file",
			content: apiBreak,
			want: []runnerapi.Diagnostic{{
				File: "logfmt_test.go", Severity: "error", Source: "compile",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needSC && !hasStaticcheck {
				t.Skip("staticcheck is not installed")
			}
			t.Parallel()
			exercise, files := cmp.Or(tt.exercise, "01-logfmt"), tt.files
			if files == nil {
				files = map[string]string{"logfmt.go": tt.content}
			}
			var resp runnerapi.CheckResponse
			req := runnerapi.CheckRequest{Exercise: exercise, Files: files}
			env.postJSON(t, runnerapi.PathCheck, req, &resp, http.StatusOK)
			if resp.Diagnostics == nil {
				t.Fatal("diagnostics is null, want a list")
			}
			if tt.exact && len(resp.Diagnostics) != len(tt.want) {
				t.Errorf("diagnostics = %+v, want exactly %d", resp.Diagnostics, len(tt.want))
			}
			for _, want := range tt.want {
				if !containsDiagnostic(resp.Diagnostics, want) {
					t.Errorf("no diagnostic like %+v in %+v", want, resp.Diagnostics)
				}
			}
			for _, d := range resp.Diagnostics {
				if d.Line < 1 || d.Column < 1 || d.Message == "" {
					t.Errorf("incomplete diagnostic %+v", d)
				}
				if tt.only != "" && d.Source != tt.only {
					t.Errorf("diagnostic from %s, want only %s: %+v", d.Source, tt.only, d)
				}
			}
		})
	}
}

// containsDiagnostic matches want's non-zero fields, so a test states only
// what it cares about.
func containsDiagnostic(diags []runnerapi.Diagnostic, want runnerapi.Diagnostic) bool {
	for _, d := range diags {
		if d.File == want.File && d.Severity == want.Severity && d.Source == want.Source && d.Code == want.Code &&
			(want.Line == 0 || d.Line == want.Line) && (want.Column == 0 || d.Column == want.Column) {
			return true
		}
	}
	return false
}

func TestRunTests(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)
	t.Cleanup(func() { assertWorkDirEmpty(t, env.workDir) })
	stub := stubFiles(t, "01-logfmt")
	typeErr := map[string]string{"logfmt.go": stub["logfmt.go"] + "\nfunc Bad() int { return \"x\" }\n"}

	tests := []struct {
		name  string
		req   runnerapi.TestRequest
		check func(t *testing.T, events []runnerapi.Event, done runnerapi.Event)
	}{
		{
			name: "stub fails",
			req:  runnerapi.TestRequest{Exercise: "01-logfmt", Files: stub, Race: true},
			check: func(t *testing.T, events []runnerapi.Event, done runnerapi.Event) {
				if done.Failed == 0 || done.ExitCode == 0 || done.BuildFailed {
					t.Errorf("done = %+v, want failed tests and a non-zero exit code", done)
				}
				if !hasOutput(events, "TODO: implement") {
					t.Error("no output with the stub's panic")
				}
				if hasOutput(events, env.workDir) {
					t.Error("output shows the workspace path")
				}
			},
		},
		{
			name: "build failure",
			req:  runnerapi.TestRequest{Exercise: "01-logfmt", Files: typeErr},
			check: func(t *testing.T, events []runnerapi.Event, done runnerapi.Event) {
				if !done.BuildFailed || done.ExitCode == 0 {
					t.Errorf("done = %+v, want buildFailed", done)
				}
				var build strings.Builder
				for _, ev := range events {
					if ev.Type == runnerapi.EventBuild {
						build.WriteString(ev.Output)
					}
				}
				if !strings.Contains(build.String(), "logfmt.go:") {
					t.Errorf("build output %q does not name logfmt.go", build.String())
				}
			},
		},
		{
			name: "01-logfmt solution passes with -race",
			req:  runnerapi.TestRequest{Exercise: "01-logfmt", Files: solutionFiles(t, "01-logfmt"), Race: true},
			check: func(t *testing.T, _ []runnerapi.Event, done runnerapi.Event) {
				if done.ExitCode != 0 || done.Failed != 0 || done.Passed == 0 || done.TimedOut || done.Truncated {
					t.Errorf("done = %+v, want a clean pass", done)
				}
			},
		},
		{
			name: "12-workerpool solution passes with -race",
			req:  runnerapi.TestRequest{Exercise: "12-workerpool", Files: solutionFiles(t, "12-workerpool"), Race: true},
			check: func(t *testing.T, _ []runnerapi.Event, done runnerapi.Event) {
				if done.ExitCode != 0 || done.Failed != 0 || done.Passed == 0 {
					t.Errorf("done = %+v, want a clean pass", done)
				}
			},
		},
		{
			// The tests listen on and dial loopback ports, which the sandbox
			// must allow.
			name: "15-healthcheck solution passes with -race",
			req:  runnerapi.TestRequest{Exercise: "15-healthcheck", Files: solutionFiles(t, "15-healthcheck"), Race: true},
			check: func(t *testing.T, _ []runnerapi.Event, done runnerapi.Event) {
				if done.ExitCode != 0 || done.Failed != 0 || done.Passed == 0 {
					t.Errorf("done = %+v, want a clean pass", done)
				}
			},
		},
		{
			name: "18-certcheck solution passes",
			req:  runnerapi.TestRequest{Exercise: "18-certcheck", Files: solutionFiles(t, "18-certcheck")},
			check: func(t *testing.T, _ []runnerapi.Event, done runnerapi.Event) {
				if done.ExitCode != 0 || done.Failed != 0 || done.Passed == 0 {
					t.Errorf("done = %+v, want a clean pass", done)
				}
			},
		},
		{
			name: "run filter",
			req:  runnerapi.TestRequest{Exercise: "01-logfmt", Files: solutionFiles(t, "01-logfmt"), Run: "TestLevel"},
			check: func(t *testing.T, events []runnerapi.Event, done runnerapi.Event) {
				if done.ExitCode != 0 || done.Passed == 0 {
					t.Errorf("done = %+v, want a pass", done)
				}
				for _, ev := range events {
					if ev.Type == runnerapi.EventTest && ev.Test != "" && !strings.HasPrefix(ev.Test, "TestLevel") {
						t.Errorf("test %s ran despite -run TestLevel", ev.Test)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events := env.runTests(t, tt.req)
			if len(events) < 2 || events[0].Type != runnerapi.EventStart && events[0].Type != runnerapi.EventQueued {
				t.Fatalf("stream starts with %+v, want start or queued", events)
			}
			tt.check(t, events, doneEvent(t, events))
		})
	}
}

func hasOutput(events []runnerapi.Event, substr string) bool {
	for _, ev := range events {
		if strings.Contains(ev.Output, substr) {
			return true
		}
	}
	return false
}

// blockingStub is the 01-logfmt stub with an init function that never
// returns, so the test binary hangs before any test runs.
func blockingStub(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"logfmt.go": stubFiles(t, "01-logfmt")["logfmt.go"] + "\nfunc init() { time.Sleep(time.Hour) }\n"}
}

func TestTimeoutKillsTheProcessGroup(t *testing.T) {
	slow(t)
	t.Parallel()
	// Long enough to build the test binary on a busy machine; the test
	// checks that the binary was running when the timeout hit.
	env := newTestEnv(t, func(o *Options) { o.TestTimeout = 20 * time.Second })

	result := runAsync(env, runnerapi.TestRequest{Exercise: "01-logfmt", Files: blockingStub(t)})
	sawBinary := waitFor(time.Minute, func() bool { return testBinaryRunning(env.workDir) })
	events := receiveWithin(t, result, requestTimeout).get(t)
	if !sawBinary {
		t.Fatal("the test binary never ran; the build may be too slow for the timeout")
	}
	done := doneEvent(t, events)
	if !done.TimedOut || done.ExitCode != -1 {
		t.Errorf("done = %+v, want timedOut and exit code -1", done)
	}
	if !waitFor(5*time.Second, func() bool { return len(processesUnder(env.workDir)) == 0 }) {
		t.Errorf("processes left behind: %v", processesUnder(env.workDir))
	}
	assertWorkDirEmpty(t, env.workDir)
	assertNoStrays(t, env)
}

func TestOutputCapKillsTheRun(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)
	files := map[string]string{"logfmt.go": stubFiles(t, "01-logfmt")["logfmt.go"] +
		"\nfunc init() {\n\tfor {\n\t\tprintln(\"" + strings.Repeat("spam ", 40) + "\")\n\t}\n}\n"}

	events := env.runTests(t, runnerapi.TestRequest{Exercise: "01-logfmt", Files: files})
	done := doneEvent(t, events)
	if !done.Truncated || done.ExitCode != -1 {
		t.Errorf("done = %+v, want truncated and exit code -1", done)
	}
	total := 0
	for _, ev := range events {
		total += len(ev.Output)
	}
	if total > runnerapi.MaxOutput {
		t.Errorf("%d bytes of output passed on, the cap is %d", total, runnerapi.MaxOutput)
	}
	if !waitFor(5*time.Second, func() bool { return len(processesUnder(env.workDir)) == 0 }) {
		t.Errorf("processes left behind: %v", processesUnder(env.workDir))
	}
}

func TestClientDisconnectKillsTheRun(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	resp := env.post(t, ctx, runnerapi.PathTest, runnerapi.TestRequest{Exercise: "01-logfmt", Files: blockingStub(t)})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !waitFor(time.Minute, func() bool { return testBinaryRunning(env.workDir) }) {
		t.Fatal("the test binary never ran")
	}
	cancel()

	if !waitFor(10*time.Second, func() bool { return len(processesUnder(env.workDir)) == 0 }) {
		t.Errorf("processes left behind after the client left: %v", processesUnder(env.workDir))
	}
	assertWorkDirEmpty(t, env.workDir)
}

func TestQueuedEventsReportThePosition(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t, func(o *Options) { o.MaxConcurrent, o.MaxQueue = 1, 1 })
	held := mustJoin(t, env.srv.limiter)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req := runnerapi.TestRequest{Exercise: "01-logfmt", Files: solutionFiles(t, "01-logfmt"), Run: "TestLevelString"}
	resp := env.post(t, ctx, runnerapi.PathTest, req)
	defer func() { _ = resp.Body.Close() }()

	first := make(chan runnerapi.Event, 1)
	result := make(chan []runnerapi.Event, 1)
	go func() {
		events, _ := readEvents(resp.Body, func(ev runnerapi.Event) {
			select {
			case first <- ev:
			default:
			}
		})
		result <- events
	}()
	if ev := receiveWithin(t, first, requestTimeout); ev.Type != runnerapi.EventQueued || ev.Position != 1 {
		t.Fatalf("first event = %+v, want queued at position 1", ev)
	}
	// The queue holds 1 request, so another one is refused now.
	var apiErr runnerapi.Error
	env.postJSON(t, runnerapi.PathTest, req, &apiErr, http.StatusTooManyRequests)

	held.Release()
	events := receiveWithin(t, result, requestTimeout)
	if len(events) < 3 || events[1].Type != runnerapi.EventStart {
		t.Fatalf("events after the queue = %+v, want start next", events)
	}
	if done := doneEvent(t, events); done.ExitCode != 0 || done.Passed == 0 {
		t.Errorf("done = %+v, want a pass", done)
	}
}

func TestCloseKillsRunningJobs(t *testing.T) {
	slow(t)
	t.Parallel()
	env := newTestEnv(t)

	result := runAsync(env, runnerapi.TestRequest{Exercise: "01-logfmt", Files: blockingStub(t)})
	if !waitFor(time.Minute, func() bool { return testBinaryRunning(env.workDir) }) {
		t.Fatal("the test binary never ran")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := env.srv.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	done := doneEvent(t, receiveWithin(t, result, requestTimeout).get(t))
	if done.ExitCode != -1 || done.TimedOut {
		t.Errorf("done = %+v, want a killed run", done)
	}
	if n := len(processesUnder(env.workDir)); n != 0 {
		t.Errorf("%d processes left after Close", n)
	}
}

// eventsOrError is the result of a test request run in a goroutine.
type eventsOrError struct {
	events []runnerapi.Event
	err    error
}

func (r eventsOrError) get(t *testing.T) []runnerapi.Event {
	t.Helper()
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.events
}

// runAsync runs a test request in a goroutine.
func runAsync(env *testEnv, req runnerapi.TestRequest) <-chan eventsOrError {
	ch := make(chan eventsOrError, 1)
	go func() {
		events, err := env.collectTests(req)
		ch <- eventsOrError{events, err}
	}()
	return ch
}

func assertWorkDirEmpty(t *testing.T, dir string) {
	t.Helper()
	var names []string
	ok := waitFor(5*time.Second, func() bool {
		names, _ = readDirNames(dir)
		return len(names) == 0
	})
	if !ok {
		t.Errorf("work dir not cleaned up: %v", names)
	}
}

// receiveWithin reads 1 value or fails the test after timeout.
func receiveWithin[T any](t *testing.T, ch <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatal("timed out")
		panic("unreachable")
	}
}
