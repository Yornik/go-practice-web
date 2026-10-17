package runner

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

var (
	// errCheckTimeout is the cause of a check that ran out of time.
	errCheckTimeout = errors.New("check timed out")
	// errTestTimeout is the cause of a test run that ran out of time.
	errTestTimeout = errors.New("test run timed out")
)

// maxLine is the longest output line passed on in 1 piece. test2json
// already splits long test output, so only a runaway line gets cut.
const maxLine = 64 << 10

// pkgPattern is the go package pattern of an exercise in a workspace.
func pkgPattern(ex *Exercise) string {
	return "./exercises/" + ex.Name + "/"
}

// runCheck runs go vet and then staticcheck on the files and returns the
// diagnostics in file and position order. It reports errCheckTimeout when
// go vet did not finish in time; a staticcheck that runs out of time is
// skipped, so that the vet results still reach the learner.
func (s *Server) runCheck(ctx context.Context, ex *Exercise, files map[string]string) ([]runnerapi.Diagnostic, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, s.opts.CheckTimeout, errCheckTimeout)
	defer cancel()

	ws, err := newWorkspace(s.opts.WorkDir, s.opts.Course, ex, files)
	if err != nil {
		return nil, err
	}
	defer s.closeWorkspace(ctx, ws)

	out, err := s.runTool(ctx, ws, s.opts.Go, nil, "vet", "-json", pkgPattern(ex))
	if err != nil {
		return nil, err
	}
	diags := parseVetOutput(out)

	if s.opts.Staticcheck != "" && !hasErrors(diags) {
		extra := []string{"STATICCHECK_CACHE=" + s.opts.StaticcheckCache}
		out, err := s.runTool(ctx, ws, s.opts.Staticcheck, extra, "-f", "json", pkgPattern(ex))
		switch {
		case errors.Is(err, errCheckTimeout):
			s.log.WarnContext(ctx, "staticcheck timed out; returning vet results", "exercise", ex.Name)
		case err != nil:
			return nil, err
		default:
			// Findings in the course's test files are not the learner's to
			// fix, so only the stub files get staticcheck's advice.
			diags = append(diags, parseStaticcheckOutput(out, ex.IsStub)...)
		}
	}
	sortDiagnostics(diags)
	return diags, nil
}

// runTool runs 1 tool in the workspace's module root and returns its
// combined output, which is capped like test output. It fails with the
// context's cause when ctx ends first.
func (s *Server) runTool(ctx context.Context, ws *workspace, path string, extraEnv []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var out bytes.Buffer
	budget := newOutputBudget(runnerapi.MaxOutput, func() { cancel(errOutputCap) })
	w := newLineWriter(budget, maxLine, func(line []byte) { out.Write(line) })
	_, err := run(ctx, command{
		path: path, args: args, dir: ws.modDir,
		env:    append(s.env(ws), extraEnv...),
		stdout: w, stderr: w,
		sandbox: s.sandbox, workspace: ws.root, onStrays: s.reportStrays,
	})
	w.Flush()
	if err != nil {
		return nil, err
	}
	// An exit code says little here: vet and staticcheck exit non-zero for
	// findings and type errors alike, which the output carries.
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, errOutputCap) {
		return nil, cause
	}
	return out.Bytes(), nil
}

// runTest runs go test -json on the files and passes every event to emit.
// It returns the done event. An error means the run could not be set up.
func (s *Server) runTest(ctx context.Context, ex *Exercise, req runnerapi.TestRequest, emit func(runnerapi.Event)) (runnerapi.Event, error) {
	start := time.Now()
	ctx, cancelCap := context.WithCancelCause(ctx)
	defer cancelCap(nil)
	ctx, cancelTimeout := context.WithTimeoutCause(ctx, s.opts.TestTimeout, errTestTimeout)
	defer cancelTimeout()

	ws, err := newWorkspace(s.opts.WorkDir, s.opts.Course, ex, req.Files)
	if err != nil {
		return runnerapi.Event{}, err
	}
	defer s.closeWorkspace(ctx, ws)

	args := testArgs(ex, req, s.opts.TestTimeout, s.opts.testTimeoutMargin)
	stream := newTestStream(ws.modDir, emit)
	budget := newOutputBudget(runnerapi.MaxOutput, func() { cancelCap(errOutputCap) })
	stdout := newLineWriter(budget, maxLine, stream.stdoutLine)
	stderr := newLineWriter(budget, maxLine, stream.stderrLine)
	exitCode, err := run(ctx, command{
		path: s.opts.Go, args: args, dir: ws.modDir,
		env: s.env(ws), stdout: stdout, stderr: stderr,
		sandbox: s.sandbox, workspace: ws.root, onStrays: s.reportStrays,
	})
	if err != nil {
		return runnerapi.Event{}, err
	}
	stdout.Flush()
	stderr.Flush()

	passed, failed, skipped, buildFailed := stream.summary()
	// A run that was killed (exit code -1) at the deadline timed out; a run
	// that ended just as the deadline passed is complete. go test's own
	// alarm makes the test binary panic, and go test then fails.
	timedOut := exitCode < 0 && errors.Is(context.Cause(ctx), errTestTimeout) ||
		exitCode != 0 && stream.alarmed()
	return runnerapi.Event{
		Type:        runnerapi.EventDone,
		ExitCode:    exitCode,
		Passed:      passed,
		Failed:      failed,
		Skipped:     skipped,
		DurationMs:  time.Since(start).Milliseconds(),
		TimedOut:    timedOut,
		Truncated:   budget.Exceeded(),
		BuildFailed: buildFailed,
	}, nil
}

// testTimeoutMargin is how much earlier than the runner's deadline go
// test's own -timeout fires: time for the build, for the test binary to
// print every goroutine and for go test to report the failure.
const testTimeoutMargin = 5 * time.Second

// goTestTimeout is the -timeout of go test for a run limited to limit.
// When it fires, the test binary panics with "test timed out after ...",
// names the tests that were running and prints every goroutine, which
// shows the learner where the code hangs. The runner's own kill at limit
// stays as the backstop, for a hang in the build, in init or in TestMain,
// before the test binary's alarm starts. The -timeout is at least a
// quarter of limit, so a short limit never gives 0, which turns it off.
func goTestTimeout(limit, margin time.Duration) time.Duration {
	return max(limit-margin, limit/4)
}

// testArgs are the go test arguments of a run with the runner's deadline
// limit and the margin before it.
func testArgs(ex *Exercise, req runnerapi.TestRequest, limit, margin time.Duration) []string {
	args := []string{"test", "-json", "-count=1", "-timeout=" + goTestTimeout(limit, margin).String()}
	if req.Race {
		args = append(args, "-race")
	}
	if req.Run != "" {
		// 1 argument with "=", so a pattern that starts with "-" stays a
		// pattern and cannot become a flag.
		args = append(args, "-run="+req.Run)
	}
	return append(args, pkgPattern(ex))
}

// closeWorkspace removes a workspace and logs a failure: the request has
// its answer either way, but a full disk needs an operator.
func (s *Server) closeWorkspace(ctx context.Context, ws *workspace) {
	if err := ws.Close(); err != nil {
		s.log.ErrorContext(ctx, "remove workspace", "dir", ws.root, "err", err)
	}
}

func hasErrors(diags []runnerapi.Diagnostic) bool {
	return slices.ContainsFunc(diags, func(d runnerapi.Diagnostic) bool {
		return d.Severity == runnerapi.SeverityError
	})
}

// sortDiagnostics orders by file, then position, keeping the tools' order
// for equal positions.
func sortDiagnostics(diags []runnerapi.Diagnostic) {
	slices.SortStableFunc(diags, func(a, b runnerapi.Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
		)
	})
}
