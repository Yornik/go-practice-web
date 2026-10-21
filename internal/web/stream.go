package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// testRun is the body of POST /api/exercises/{exercise}/test: a step ID,
// or a -run regular expression, and whether to use -race. Without either
// it runs every test.
type testRun struct {
	Step string  `json:"step"`
	Run  *string `json:"run"`
	Race *bool   `json:"race"`
}

// maxRunRegexp bounds a hand-written -run filter.
const maxRunRegexp = 512

// resolvedRun is what the runner gets and how the result counts.
type resolvedRun struct {
	Step string // the step ID, or "" for a hand-written filter
	Run  string
	Race bool // the run uses -race
	// RaceUnavailable is true when the runner's race detector does not
	// work: Race is then false whatever the learner chose.
	RaceUnavailable bool
}

// all reports whether the run is the full run: no filter. Only a full run
// can pass an exercise.
func (r resolvedRun) all() bool { return r.Run == "" }

// canPass reports whether a clean run passes the exercise: a full run with
// -race, or without it on a runner whose race detector does not work,
// where no run can do better.
func (r resolvedRun) canPass() bool { return r.all() && (r.Race || r.RaceUnavailable) }

func resolveRun(ex *course.Exercise, in testRun) (resolvedRun, error) {
	var rr resolvedRun
	switch {
	case in.Step != "" && in.Run != nil:
		return rr, errorf(http.StatusBadRequest, "bad_run", "Send a step or a -run filter, not both.")
	case in.Step != "":
		st, ok := ex.Step(in.Step)
		if !ok {
			return rr, errorf(http.StatusBadRequest, "unknown_step", "This exercise has no step "+in.Step+".")
		}
		rr = resolvedRun{Step: st.ID, Run: st.Run, Race: st.Race}
	case in.Run != nil:
		if len(*in.Run) > maxRunRegexp {
			return rr, errorf(http.StatusBadRequest, "bad_run", "The -run filter is too long.")
		}
		if _, err := regexp.Compile(*in.Run); err != nil {
			return rr, errorf(http.StatusBadRequest, "bad_run", "The -run filter is not a valid regular expression: "+err.Error())
		}
		rr = resolvedRun{Run: *in.Run, Race: true}
	default:
		all := ex.AllTests()
		rr = resolvedRun{Step: all.ID, Run: all.Run, Race: all.Race}
	}
	if in.Race != nil {
		rr.Race = *in.Race
	}
	return rr, nil
}

// Events the web server adds to the runner's stream.
const (
	eventExplain  = "explain"
	eventProgress = "progress"
	eventError    = "error"
)

// explainEvent teaches what went wrong in a failed test or build.
type explainEvent struct {
	Type  string                `json:"type"`
	Test  string                `json:"test"` // "" for the build or package-level output
	Items []explain.Explanation `json:"items"`
}

// progressEvent ends every stream that reached the runner's done event.
type progressEvent struct {
	Type   string `json:"type"`
	Status string `json:"status"` // the exercise: "started" or "passed"
	// AllTests: the run had no filter and -race, or no filter on a runner
	// without a race detector. It can pass the exercise.
	AllTests      bool      `json:"allTests"`
	Passed        bool      `json:"passed"`    // this run passed with zero failures
	FirstPass     bool      `json:"firstPass"` // this run passed the exercise for the first time
	PassedAt      time.Time `json:"passedAt,omitzero"`
	Runs          int       `json:"runs"`
	ActiveSeconds int64     `json:"activeSeconds"`
	// RaceUnavailable: the race detector does not work on the runner, so
	// the run went without -race. The IDE says so next to the race toggle.
	RaceUnavailable bool `json:"raceUnavailable,omitempty"`
}

// errorEvent ends a stream that cannot reach progress: the runner stream
// broke after it started (runner_stream), or the run never started because
// it waited too long in the runner's queue (runner_busy), or the site
// could not save the run (internal).
type errorEvent struct {
	Type  string `json:"type"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

// runSummary is the last_result the store keeps for the dashboard.
type runSummary struct {
	Step        string `json:"step,omitempty"`
	Run         string `json:"run,omitempty"`
	Race        bool   `json:"race"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	Skipped     int    `json:"skipped"`
	ExitCode    int    `json:"exitCode"`
	DurationMs  int64  `json:"durationMs"`
	TimedOut    bool   `json:"timedOut,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	BuildFailed bool   `json:"buildFailed,omitempty"`
	// RaceUnavailable: the runner had no race detector, so Race is false.
	RaceUnavailable bool      `json:"raceUnavailable,omitempty"`
	At              time.Time `json:"at"`
	// Tests is the result (pass, fail or skip) of each top-level test, and
	// FailedTests every failed subtest, so the IDE can show the last run's
	// tree after a reload. The output and the explanations are not kept.
	Tests       map[string]string `json:"tests,omitempty"`
	FailedTests []string          `json:"failedTests,omitempty"`
}

// maxSummaryTests bounds the tests a summary keeps: a fuzz target with many
// seeds or a large table must not grow a progress row without limit.
const maxSummaryTests = 200

// summaryTests returns the top-level results and the failed subtests of a
// run. The tests that never reported a result get the state the IDE gives
// them when the run ends, so a reload still shows the test that hung or
// exited.
func (t *runTracker) summaryTests(done runnerapi.Event) (map[string]string, []string) {
	tops := make(map[string]string, len(t.results))
	for name, r := range t.results {
		if len(tops) >= maxSummaryTests {
			break
		}
		tops[name] = r
	}
	var failed []string
	for name := range t.failures {
		if strings.Contains(name, "/") {
			failed = append(failed, name)
		}
	}
	state := unfinishedState(done)
	for _, name := range t.unfinished() {
		switch {
		case !strings.Contains(name, "/"):
			if _, ok := tops[name]; !ok && len(tops) < maxSummaryTests {
				tops[name] = state
			}
		case state == "fail":
			failed = append(failed, name)
		}
	}
	slices.Sort(failed)
	if len(failed) > maxSummaryTests {
		failed = failed[:maxSummaryTests]
	}
	return tops, failed
}

// unfinishedState is the result of a test that never reported one. It
// failed when go test's -timeout fired or the test binary died (an exit or
// a crash); it is skipped when the run was stopped or cut off from outside.
func unfinishedState(done runnerapi.Event) string {
	if done.TimedOut || done.ExitCode > 0 {
		return "fail"
	}
	return "skip"
}

// runPassed reports whether a done event is a clean pass.
func runPassed(d runnerapi.Event) bool {
	return d.ExitCode == 0 && d.Failed == 0 && d.Passed > 0 && !d.BuildFailed && !d.TimedOut && !d.Truncated
}

func (s *Server) startRun(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[userID] {
		return false
	}
	s.running[userID] = true
	return true
}

func (s *Server) endRun(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, userID)
}

// runTests proxies a test run: the runner's NDJSON events go to the
// browser as they come, with explanations after failures and a final
// progress event. The run uses the saved files, so the browser saves
// before it runs.
func (s *Server) runTests(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var in testRun
	if err := decode(w, r, maxJSON, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	rr, err := resolveRun(ex, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, _ := identity(r)
	userID := id.User.ID
	if !s.startRun(userID) {
		s.fail(w, r, errorf(http.StatusConflict, "run_in_progress", "A test run is still going. Wait for it or stop it first."))
		return
	}
	defer s.endRun(userID)
	if err := limit(s.testLimit, userID); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	files, err := s.stubContents(ctx, userID, ex)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A runner without a race detector would drop -race itself; asking
	// without it keeps the request honest about what runs.
	raceDropped := false
	if s.raceUnavailable(ctx) {
		rr.RaceUnavailable = true
		raceDropped, rr.Race = rr.Race, false
	}
	stream, err := s.cfg.Runner.Test(ctx, runnerapi.TestRequest{Exercise: ex.Slug, Files: files, Run: rr.Run, Race: rr.Race})
	if err != nil {
		if re := (*RunnerError)(nil); errors.As(err, &re) && re.Code == "runner_unavailable" {
			s.forgetRunnerInfo()
		}
		s.runnerFailed(w, r, err)
		return
	}
	defer func() { _ = stream.Close() }()

	h := w.Header()
	h.Set("Content-Type", "application/x-ndjson")
	h.Set("Cache-Control", "no-store")
	// Proxies must pass each line on at once.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	out := newEventWriter(w)

	tracker := newRunTracker(s.cfg.Course, ex)
	var done *runnerapi.Event
	for stream.Next() {
		ev := stream.Event()
		if ev.Type == runnerapi.EventDone && ev.Busy {
			// The run never started: it waited too long in the runner's
			// queue. Nothing ran, so nothing is counted; the IDE shows the
			// message like any other error of the stream.
			_ = out.send(errorEvent{Type: eventError, Code: "runner_busy",
				Error: "The test runner is busy. Try again in a few seconds."})
			return
		}
		var extra []any
		if ev.Type == runnerapi.EventDone {
			if raceDropped {
				ev.RaceUnavailable = true
			}
			if ev.RaceUnavailable {
				rr.Race, rr.RaceUnavailable = false, true
				if !raceDropped {
					s.noteRaceUnavailable()
				}
			}
			// Explanations of the build and of package-level output go
			// before done, so the UI has them when it shows the summary.
			for _, x := range tracker.finish(ev) {
				if out.send(x) != nil {
					return
				}
			}
			d := ev
			done = &d
		} else {
			extra = tracker.observe(ev)
		}
		if out.send(ev) != nil {
			return // the browser left; the context cancels the run
		}
		for _, x := range extra {
			if out.send(x) != nil {
				return
			}
		}
	}
	if err := stream.Err(); err != nil || done == nil {
		if ctx.Err() != nil {
			return
		}
		s.forgetRunnerInfo()
		s.log.Warn("test stream broke", "exercise", ex.Slug, "err", err)
		_ = out.send(errorEvent{Type: eventError, Code: "runner_stream",
			Error: "The connection to the test runner broke during the run. Try again."})
		return
	}
	// A full run must report every test of the exercise. Learner code runs
	// inside the test binary and can print test2json lines of its own, so a
	// clean done event alone is easy to fake; this catches the cheap fakes
	// and runs that ended early in any other way.
	complete := true
	if rr.all() {
		if missing := tracker.missing(requiredTests(ex)); len(missing) > 0 {
			complete = false
			if runPassed(*done) {
				s.log.Warn("a clean full run did not report every test", "exercise", ex.Slug,
					"user_id", userID, "missing", len(missing))
			}
		}
	}
	// The run happened even if the browser left just now: count it.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	tops, failed := tracker.summaryTests(*done)
	p, err := s.recordRun(rctx, userID, ex.Slug, rr, *done, complete, tops, failed)
	if err != nil {
		s.log.Error("record run", "exercise", ex.Slug, "err", err)
		_ = out.send(errorEvent{Type: eventError, Code: "internal", Error: "The run finished, but the site could not save your progress."})
		return
	}
	_ = out.send(p)
}

// recordRun counts the run, keeps its summary and passes the exercise
// when this was a clean full run with -race (or on a runner without a race
// detector) that reported every test (complete).
func (s *Server) recordRun(ctx context.Context, userID int64, slug string, rr resolvedRun, d runnerapi.Event, complete bool, tests map[string]string, failed []string) (progressEvent, error) {
	sum, err := json.Marshal(runSummary{
		Step: rr.Step, Run: rr.Run, Race: rr.Race, Passed: d.Passed, Failed: d.Failed, Skipped: d.Skipped,
		ExitCode: d.ExitCode, DurationMs: d.DurationMs, TimedOut: d.TimedOut, Truncated: d.Truncated,
		BuildFailed: d.BuildFailed, RaceUnavailable: rr.RaceUnavailable, At: s.now().UTC(),
		Tests: tests, FailedTests: failed,
	})
	if err != nil {
		return progressEvent{}, err
	}
	if err := s.cfg.Store.RecordRun(ctx, userID, slug, sum); err != nil {
		return progressEvent{}, err
	}
	ev := progressEvent{Type: eventProgress, AllTests: rr.canPass(), Passed: runPassed(d) && complete,
		RaceUnavailable: rr.RaceUnavailable}
	if ev.AllTests && ev.Passed {
		if ev.FirstPass, err = s.cfg.Store.MarkPassed(ctx, userID, slug); err != nil {
			return progressEvent{}, err
		}
	}
	p, err := s.cfg.Store.Progress(ctx, userID, slug)
	if err != nil {
		return progressEvent{}, err
	}
	ev.Status, ev.PassedAt, ev.Runs, ev.ActiveSeconds = string(p.Status), p.PassedAt, p.Runs, p.ActiveSeconds
	return ev, nil
}

// requiredTests are the top-level tests and fuzz targets of the exercise:
// a full run reports a result for each. Examples are left out, because
// one without an output comment is compiled but never run.
func requiredTests(ex *course.Exercise) []string {
	var out []string
	for _, name := range ex.Tests {
		if strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Test") && name != "TestMain" {
			out = append(out, name)
		}
	}
	return out
}

// eventWriter writes NDJSON lines and flushes each one. Every write has a
// deadline, so a browser that stops reading cannot hold the run open.
type eventWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

const eventWriteTimeout = 30 * time.Second

func newEventWriter(w http.ResponseWriter) *eventWriter {
	return &eventWriter{w: w, rc: http.NewResponseController(w)}
}

func (e *eventWriter) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Not every ResponseWriter (httptest) supports deadlines.
	if err := e.rc.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := e.w.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := e.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}

// runTracker collects test output during a run and explains failures.
//
// go test -json reports a subtest's failure before its parent's, but a
// panic belongs to the top-level test. So a subtest whose own output
// explains nothing specific waits: if its parent then has a specific
// explanation (the stub's "TODO: implement" panic), that one is enough;
// otherwise the subtest gets the generic guide. A parent whose output
// shows nothing but its own "--- FAIL" line failed only because a subtest
// did; the subtests' explanations say why, so the parent gets none.
type runTracker struct {
	course    *course.Course                   // names the exercise an explanation points to
	ex        *course.Exercise                 // its files are the ones the editor can open
	results   map[string]string                // top-level test: its last action (pass, fail or skip)
	started   map[string]bool                  // every test that reported run
	ended     map[string]bool                  // every test that reported pass, fail or skip
	failures  map[string]bool                  // every test that failed
	output    map[string]*strings.Builder      // by test name; "" is package output
	explained map[string]bool                  // tests that got an explain event
	pending   map[string][]explain.Explanation // subtests with only a generic guide
	guides    map[string]bool                  // generic guides sent; once per run is enough
	build     strings.Builder
}

// maxTrackedOutput bounds the output kept per test. The runner caps a run
// at 1 MiB, so this only matters for 1 test that prints a lot; the start
// of the output is the part that explains the failure.
const maxTrackedOutput = 256 << 10

func newRunTracker(c *course.Course, ex *course.Exercise) *runTracker {
	return &runTracker{
		course:    c,
		ex:        ex,
		results:   map[string]string{},
		started:   map[string]bool{},
		ended:     map[string]bool{},
		failures:  map[string]bool{},
		output:    map[string]*strings.Builder{},
		explained: map[string]bool{},
		pending:   map[string][]explain.Explanation{},
		guides:    map[string]bool{},
	}
}

func appendCapped(b *strings.Builder, s string) {
	if room := maxTrackedOutput - b.Len(); room > 0 {
		b.WriteString(s[:min(len(s), room)])
	}
}

// observe records an event and returns the events to send after it.
func (t *runTracker) observe(ev runnerapi.Event) []any {
	switch ev.Type {
	case runnerapi.EventBuild:
		appendCapped(&t.build, ev.Output)
	case runnerapi.EventTest:
		switch ev.Action {
		case "run":
			t.started[ev.Test] = true
		case "output":
			b, ok := t.output[ev.Test]
			if !ok {
				b = &strings.Builder{}
				t.output[ev.Test] = b
			}
			appendCapped(b, ev.Output)
		case "pass", "skip":
			t.result(ev)
		case "fail":
			if ev.Test == "" {
				return nil // package failure: finish explains it
			}
			t.result(ev)
			t.failures[ev.Test] = true
			return t.failed(ev.Test)
		}
	}
	return nil
}

// result records the outcome of a test; results keeps the top-level ones.
func (t *runTracker) result(ev runnerapi.Event) {
	if ev.Test == "" {
		return
	}
	t.ended[ev.Test] = true
	if !strings.Contains(ev.Test, "/") {
		t.results[ev.Test] = ev.Action
	}
}

// missing returns the tests of names that did not pass or skip.
func (t *runTracker) missing(names []string) []string {
	var out []string
	for _, n := range names {
		if r := t.results[n]; r != "pass" && r != "skip" {
			out = append(out, n)
		}
	}
	return out
}

// failed explains 1 failed test from its own output.
func (t *runTracker) failed(test string) []any {
	var out string
	if b, ok := t.output[test]; ok {
		out = b.String()
	}
	items := explain.ExplainTestOutput(test, out)
	if t.failedOnlyBelow(test, items) {
		items = nil
	}
	var specific []explain.Explanation
	for _, it := range items {
		if !it.Generic {
			specific = append(specific, it)
		}
	}
	if strings.Contains(test, "/") && len(specific) == 0 {
		if len(items) > 0 {
			t.pending[test] = items
		}
		return nil
	}
	if len(specific) > 0 {
		t.dropPending(test)
		if t.childExplained(test) {
			items = specific
		}
		return t.emit(test, items)
	}
	if evs := t.releasePending(test + "/"); len(evs) > 0 {
		return evs
	}
	if t.childExplained(test) {
		return nil
	}
	return t.emit(test, items)
}

// failedOnlyBelow reports whether test failed only because a subtest
// failed: a subtest failed, and all its own output explains is its
// "--- FAIL" line, which the generic guide "How to read a failing test"
// stands for.
func (t *runTracker) failedOnlyBelow(test string, items []explain.Explanation) bool {
	if len(items) == 0 || !t.hasFailedSubtest(test) {
		return false
	}
	for _, it := range items {
		if it.ID != explain.GuideFailingTest || it.Raw.File != "" {
			return false
		}
	}
	return true
}

func (t *runTracker) hasFailedSubtest(test string) bool {
	for name := range t.failures {
		if strings.HasPrefix(name, test+"/") {
			return true
		}
	}
	return false
}

// emit returns the explain event for test, without the generic guides
// that went out before.
func (t *runTracker) emit(test string, items []explain.Explanation) []any {
	var kept []explain.Explanation
	for _, it := range items {
		if it.Generic {
			if t.guides[it.ID] {
				continue
			}
			t.guides[it.ID] = true
		}
		kept = append(kept, it)
	}
	if len(kept) == 0 {
		return nil
	}
	t.explained[test] = true
	return []any{t.event(test, kept)}
}

// event is the explain event for test with items, which it marks openable
// where the editor can open their file.
func (t *runTracker) event(test string, items []explain.Explanation) explainEvent {
	prepare(t.course, t.ex, items)
	return explainEvent{Type: eventExplain, Test: test, Items: items}
}

// prepare readies explanations for the IDE. It sets Raw.Openable on those
// whose file is a stub or test file of the exercise: a file of the runtime
// or the standard library, such as panic.go in a recovered runtime error,
// stays plain text in the UI, because the editor has no such file. And it
// names each course exercise by the slug the loaded course uses.
func prepare(c *course.Course, ex *course.Exercise, items []explain.Explanation) {
	for i := range items {
		if c != nil {
			items[i] = items[i].WithCourse(c.CurrentSlug(items[i].Course))
		}
		raw := &items[i].Raw
		if ex == nil || raw.File == "" {
			raw.Openable = false
			continue
		}
		_, raw.Openable = ex.File(raw.File)
	}
}

func (t *runTracker) childExplained(test string) bool {
	for name := range t.explained {
		if strings.HasPrefix(name, test+"/") {
			return true
		}
	}
	return false
}

func (t *runTracker) dropPending(test string) {
	for name := range t.pending {
		if strings.HasPrefix(name, test+"/") {
			delete(t.pending, name)
		}
	}
}

// releasePending emits the waiting subtests whose names start with
// prefix, in name order.
func (t *runTracker) releasePending(prefix string) []any {
	var names []string
	for name := range t.pending {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var out []any
	for _, name := range names {
		out = append(out, t.emit(name, t.pending[name])...)
		delete(t.pending, name)
	}
	return out
}

// finish returns the explanations that need the whole run: the build, the
// tests that were still running when the test binary died, or package
// output of a run that failed without a failed test (a panic in init, a
// TestMain that exits).
func (t *runTracker) finish(done runnerapi.Event) []any {
	// A killed run never reports the parents of waiting subtests.
	out := t.releasePending("")
	if t.build.Len() > 0 {
		if _, items := explain.ExplainBuildOutput(t.build.String()); len(items) > 0 {
			return append(out, t.event("", items))
		}
	}
	if done.ExitCode == 0 {
		return out
	}
	// Cut-off output, or a run killed before it ended (stopped, or ended
	// by the output cap), leaves the last test's output incomplete: an
	// explanation of it would guess.
	if done.Truncated || done.ExitCode < 0 && !done.TimedOut {
		return out
	}
	// When go test's -timeout fires, the test binary panics and no test
	// reports a result: the panic and the goroutines are in the output of
	// the test that was running. A crash looks the same.
	unfinished := t.unfinished()
	for _, name := range unfinished {
		var output string
		if b, ok := t.output[name]; ok {
			output = b.String()
		}
		items := explain.ExplainTestOutput(name, output)
		// The deepest unfinished test is where the run ended; its parents
		// only waited in t.Run. A parent keeps its own messages, such as a
		// got/want it logged first, but "the run ended here" is wrong for it.
		parent := slices.ContainsFunc(unfinished, func(n string) bool { return strings.HasPrefix(n, name+"/") })
		// A run the runner killed at its own deadline (exit code -1) did not
		// exit by itself: output that stops after a printed line is a hang,
		// not os.Exit or log.Fatal.
		killed := done.ExitCode < 0
		if parent || killed {
			items = slices.DeleteFunc(items, func(e explain.Explanation) bool {
				return strings.HasPrefix(e.ID, "runtime-exit") || parent && e.ID == "test-timeout"
			})
		}
		out = append(out, t.emit(name, items)...)
	}
	if len(t.explained) > 0 {
		return out
	}
	var pkg string
	if b, ok := t.output[""]; ok {
		pkg = b.String()
	}
	if items := explain.ExplainTestOutput("", pkg); len(items) > 0 {
		out = append(out, t.event("", items))
	}
	return out
}

// unfinished lists the tests that started but never reported a result, in
// name order.
func (t *runTracker) unfinished() []string {
	var names []string
	for name := range t.started {
		if name != "" && !t.ended[name] {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
