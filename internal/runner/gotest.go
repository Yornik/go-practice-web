package runner

import (
	"encoding/json"
	"iter"
	"strings"
	"sync"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// testJSONEvent is one line of go test -json output (cmd/test2json), plus
// the build events that go test -json prints since Go 1.24.
type testJSONEvent struct {
	Action      string  `json:"Action"`
	Package     string  `json:"Package"`
	Test        string  `json:"Test"`
	Elapsed     float64 `json:"Elapsed"`
	Output      string  `json:"Output"`
	FailedBuild string  `json:"FailedBuild"`
}

// forwardedActions are the go test -json actions the runner passes on. The
// others (start, pause, cont, bench) do not change what a learner sees.
var forwardedActions = map[string]bool{
	"run": true, "pass": true, "fail": true, "skip": true, "output": true,
}

// testStream turns go test -json output into runner events and keeps the
// summary. Its methods are called from the stdout and stderr copying
// goroutines, so it serialises them: events also leave in order.
type testStream struct {
	mu          sync.Mutex
	emit        func(runnerapi.Event)
	trim        *strings.Replacer
	results     map[string]string // test name -> last of pass, fail or skip
	buildFailed bool
}

// newTestStream returns a testStream that passes events to emit. modDir is
// the workspace's module root; it is cut from output so that stack traces
// show exercises/<ex>/file.go:12 instead of a random temporary path.
func newTestStream(modDir string, emit func(runnerapi.Event)) *testStream {
	return &testStream{
		emit:    emit,
		trim:    strings.NewReplacer(modDir+"/", ""),
		results: make(map[string]string),
	}
}

// stdoutLine handles 1 line of go test -json output.
func (s *testStream) stdoutLine(line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ev testJSONEvent
	if err := json.Unmarshal(line, &ev); err != nil || ev.Action == "" {
		// Not test2json output; pass it on as package output rather than
		// hide it.
		s.emit(runnerapi.Event{Type: runnerapi.EventTest, Action: "output", Output: s.trim.Replace(string(line))})
		return
	}
	switch ev.Action {
	case "build-output":
		s.emit(runnerapi.Event{Type: runnerapi.EventBuild, Output: s.trim.Replace(ev.Output)})
		return
	case "build-fail":
		s.buildFailed = true
		return
	}
	if ev.FailedBuild != "" {
		s.buildFailed = true
	}
	if !forwardedActions[ev.Action] {
		return
	}
	if ev.Test != "" && ev.Action != "run" && ev.Action != "output" {
		s.results[ev.Test] = ev.Action
	}
	s.emit(runnerapi.Event{
		Type:    runnerapi.EventTest,
		Action:  ev.Action,
		Test:    ev.Test,
		Elapsed: ev.Elapsed,
		Output:  s.trim.Replace(ev.Output),
	})
}

// stderrLine handles 1 line that go test wrote on stderr. With -json that
// is only the go command's own complaints (a broken go.mod, a bad flag),
// which belong with the build output.
func (s *testStream) stderrLine(line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit(runnerapi.Event{Type: runnerapi.EventBuild, Output: s.trim.Replace(string(line))})
}

// summary returns the counts and whether the build failed.
//
// The counts are about leaf tests: a parent test fails whenever 1 of its
// subtests fails, and counting both would report 2 failures for 1 mistake.
// A parent that failed while all its subtests passed (an error in the
// parent itself) still counts as a failure, so failed is never 0 for a run
// in which a test failed.
func (s *testStream) summary() (passed, failed, skipped int, buildFailed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hasChildren := make(map[string]bool)
	failedBelow := make(map[string]bool)
	for name, result := range s.results {
		for parent := range ancestors(name) {
			hasChildren[parent] = true
			if result == "fail" {
				failedBelow[parent] = true
			}
		}
	}
	for name, result := range s.results {
		switch {
		case hasChildren[name] && (result != "fail" || failedBelow[name]):
			// Counted through its subtests.
		case result == "pass":
			passed++
		case result == "fail":
			failed++
		case result == "skip":
			skipped++
		}
	}
	return passed, failed, skipped, s.buildFailed
}

// ancestors yields the parent test names of a subtest name:
// "A/b/c" gives "A/b" and "A".
func ancestors(name string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for i := strings.LastIndexByte(name, '/'); i > 0; i = strings.LastIndexByte(name[:i], '/') {
			if !yield(name[:i]) {
				return
			}
		}
	}
}
