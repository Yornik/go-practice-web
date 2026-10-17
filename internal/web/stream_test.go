package web_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// runTest posts a test run for 01-logfmt and returns the events of the
// stream by type: the last of each type, and all explain events in order.
func (h *harness) runTest(as *user, body map[string]any) (last map[string]map[string]any, explains []map[string]any, types []string) {
	h.t.Helper()
	w := h.do(req{method: "POST", path: "/api/exercises/01-logfmt/test", body: body, as: as})
	wantStatus(h.t, w, http.StatusOK)
	last = map[string]map[string]any{}
	for _, ev := range ndjson(h.t, w.Body.String()) {
		typ := ev["type"].(string)
		types = append(types, typ)
		last[typ] = ev
		if typ == "explain" {
			explains = append(explains, ev)
		}
	}
	return last, explains, types
}

// withDone replaces the done event of a run.
func withDone(events func(runnerapi.TestRequest) []runnerapi.Event, change func(*runnerapi.Event)) func(runnerapi.TestRequest) []runnerapi.Event {
	return func(req runnerapi.TestRequest) []runnerapi.Event {
		evs := events(req)
		change(&evs[len(evs)-1])
		return evs
	}
}

func TestRaceUnavailableOnTheRunner(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.setInfo(&runnerapi.Info{GoVersion: "go1.27.2", RaceError: "FATAL: ThreadSanitizer: unexpected memory mapping"}, 0)
	h.runner.events = passingRun
	raceUnavailable := func() any {
		t.Helper()
		w := h.do(req{method: "GET", path: "/api/exercises/01-logfmt", as: admin})
		wantStatus(t, w, http.StatusOK)
		return decodeJSON[map[string]any](t, w)["raceUnavailable"]
	}
	// Opening the IDE does not wait on the runner: before any run the
	// site does not know yet.
	if got := raceUnavailable(); got != false || h.runner.infoCount() != 0 {
		t.Fatalf("bootstrap before a run: raceUnavailable = %v after %d info calls", got, h.runner.infoCount())
	}

	// A step with -race: the runner gets no -race, and the stream says why.
	last, _, _ := h.runTest(admin, map[string]any{"step": "step-1", "race": true})
	if got := raceUnavailable(); got != true {
		t.Errorf("bootstrap after a run without a race detector: raceUnavailable = %v", got)
	}
	if got := h.runner.lastTest(); got.Race {
		t.Fatalf("runner request = %+v, want no -race", got)
	}
	if last["done"]["raceUnavailable"] != true {
		t.Errorf("done = %v, want raceUnavailable", last["done"])
	}
	if p := last["progress"]; p["raceUnavailable"] != true || p["allTests"] != false || p["status"] != "started" {
		t.Errorf("progress after a step = %v", p)
	}

	// A full run without -race passes the exercise: no run can do better
	// on this runner.
	last, _, _ = h.runTest(admin, map[string]any{"step": "all", "race": true})
	if p := last["progress"]; p["allTests"] != true || p["passed"] != true || p["firstPass"] != true ||
		p["status"] != "passed" || p["raceUnavailable"] != true {
		t.Fatalf("progress after all tests = %v", p)
	}
	if n := h.runner.infoCount(); n != 1 {
		t.Errorf("GET /v1/info was asked %d times for 2 runs, want 1", n)
	}

	// After the cache time the site asks again; the new runner pod has a
	// race detector, and runs use it again.
	h.runner.setInfo(&runnerapi.Info{GoVersion: "go1.27.2", RaceAvailable: true}, 0)
	h.clock.Advance(6 * time.Minute)
	last, _, _ = h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); !got.Race {
		t.Fatalf("runner request = %+v, want -race", got)
	}
	if last["progress"]["raceUnavailable"] != nil || last["done"]["raceUnavailable"] != nil {
		t.Errorf("a run with -race says raceUnavailable: %v %v", last["done"], last["progress"])
	}
	if got := raceUnavailable(); got != false {
		t.Errorf("bootstrap after a run with -race: raceUnavailable = %v", got)
	}
	if n := h.runner.infoCount(); n != 2 {
		t.Errorf("GET /v1/info was asked %d times, want 2", n)
	}
}

func TestRaceAvailableAgain(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.setInfo(&runnerapi.Info{GoVersion: "go1.27.2", RaceError: "FATAL: ThreadSanitizer: unexpected memory mapping"}, 0)
	h.runner.events = passingRun

	h.runTest(admin, map[string]any{"step": "step-1"})
	if got := h.runner.lastTest(); got.Race {
		t.Fatalf("runner request = %+v, want no -race", got)
	}
	// A new runner pod with a race detector, without a failed call in
	// between: a rollout moves the Service to the new pod.
	h.runner.setInfo(&runnerapi.Info{GoVersion: "go1.27.2", RaceAvailable: true}, 0)
	h.clock.Advance(20 * time.Second)
	h.runTest(admin, map[string]any{"step": "step-1"})
	if got, n := h.runner.lastTest(), h.runner.infoCount(); got.Race || n != 1 {
		t.Fatalf("20 s later: runner request = %+v after %d info calls, want the kept answer", got, n)
	}
	// An answer without a race detector is kept for 30 s, not 5 minutes:
	// meanwhile a full run without -race could pass the exercise.
	h.clock.Advance(15 * time.Second)
	last, _, _ := h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); !got.Race {
		t.Fatalf("35 s later: runner request = %+v, want -race again", got)
	}
	if p := last["progress"]; p["raceUnavailable"] != nil || p["allTests"] != true || p["passed"] != true {
		t.Errorf("progress = %v, want a pass with -race", p)
	}
	if n := h.runner.infoCount(); n != 2 {
		t.Errorf("GET /v1/info was asked %d times, want 2", n)
	}
}

func TestRunnerDropsRaceItself(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	// The site's answer is out of date: the runner runs without -race and
	// says so in its done event.
	h.runner.events = withDone(passingRun, func(d *runnerapi.Event) { d.RaceUnavailable = true })

	last, _, _ := h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); !got.Race {
		t.Fatalf("first runner request = %+v, want -race", got)
	}
	if p := last["progress"]; p["allTests"] != true || p["passed"] != true || p["raceUnavailable"] != true {
		t.Fatalf("progress = %v, want a pass of all tests without the race detector", p)
	}
	// The next run asks without -race, without asking the runner again.
	h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); got.Race {
		t.Fatalf("second runner request = %+v, want no -race", got)
	}
	if n := h.runner.infoCount(); n != 1 {
		t.Errorf("GET /v1/info was asked %d times, want 1", n)
	}
}

func TestRunnerInfoFailure(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.setInfo(nil, http.StatusNotFound)
	h.runner.events = passingRun

	// Without an answer the site asks for -race, as before; a runner
	// without a race detector drops it itself.
	last, _, _ := h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); !got.Race || last["progress"]["raceUnavailable"] != nil {
		t.Fatalf("runner request = %+v, progress %v", got, last["progress"])
	}
	// A failed fetch is not repeated for every run...
	h.runTest(admin, map[string]any{"step": "step-1"})
	if n := h.runner.infoCount(); n != 1 {
		t.Errorf("GET /v1/info was asked %d times right after a failure, want 1", n)
	}
	// ...but a few seconds later.
	h.runner.setInfo(&runnerapi.Info{GoVersion: "go1.27.2"}, 0)
	h.clock.Advance(11 * time.Second)
	h.runTest(admin, map[string]any{})
	if got := h.runner.lastTest(); got.Race {
		t.Errorf("runner request = %+v, want no -race once the runner said so", got)
	}
	if n := h.runner.infoCount(); n != 2 {
		t.Errorf("GET /v1/info was asked %d times, want 2", n)
	}
}

func TestBusyRunnerEndsTheStream(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
		return []runnerapi.Event{
			{Type: runnerapi.EventQueued, Position: 3},
			{Type: runnerapi.EventQueued, Position: 2},
			{Type: runnerapi.EventDone, ExitCode: -1, Busy: true},
		}
	}
	last, _, types := h.runTest(admin, map[string]any{})
	if got, want := types, []string{"start", "queued", "queued", "error"}; !equalStrings(got, want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
	if e := last["error"]; e["code"] != "runner_busy" || e["error"] == "" {
		t.Errorf("error event = %v", e)
	}
	// The run never started, so it is not counted.
	if p, err := h.store.Progress(t.Context(), admin.ID, "01-logfmt"); err == nil && p.Runs != 0 {
		t.Errorf("runs = %d, want 0", p.Runs)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func output(test, text string) runnerapi.Event {
	return runnerapi.Event{Type: runnerapi.EventTest, Action: "output", Test: test, Output: text}
}

func action(act, test string) runnerapi.Event {
	return runnerapi.Event{Type: runnerapi.EventTest, Action: act, Test: test}
}

func TestStreamExplanations(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	items := func(ev map[string]any) []map[string]any {
		var out []map[string]any
		for _, it := range ev["items"].([]any) {
			out = append(out, it.(map[string]any))
		}
		return out
	}

	t.Run("a parent that failed only through its subtests", func(t *testing.T) {
		// Real go test -json output of a nested table test. TestParse/a and
		// TestParse print only their own "--- FAIL" line.
		h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
			return []runnerapi.Event{
				action("run", "TestParse"),
				output("TestParse", "=== RUN   TestParse\n"),
				action("run", "TestParse/a"),
				output("TestParse/a", "=== RUN   TestParse/a\n"),
				action("run", "TestParse/a/b"),
				output("TestParse/a/b", "=== RUN   TestParse/a/b\n"),
				output("TestParse/a/b", "    logfmt_test.go:7: Parse(\"a\") = 1, want 2\n"),
				output("TestParse/a/b", "--- FAIL: TestParse/a/b (0.00s)\n"),
				action("fail", "TestParse/a/b"),
				output("TestParse/a", "--- FAIL: TestParse/a (0.00s)\n"),
				action("fail", "TestParse/a"),
				output("TestParse", "--- FAIL: TestParse (0.00s)\n"),
				action("fail", "TestParse"),
				{Type: runnerapi.EventDone, ExitCode: 1, Failed: 1},
			}
		}
		h.clock.Advance(time.Minute) // the test run limit
		_, explains, _ := h.runTest(admin, map[string]any{})
		if len(explains) != 1 || explains[0]["test"] != "TestParse/a/b" {
			t.Fatalf("explain events = %v, want 1 for TestParse/a/b", explains)
		}
		it := items(explains[0])[0]
		raw := it["raw"].(map[string]any)
		if it["id"] != "test-got-want" || raw["file"] != "logfmt_test.go" || raw["openable"] != true {
			t.Errorf("explanation = %v, want test-got-want in logfmt_test.go, openable", it)
		}
	})

	t.Run("a runtime file is not openable", func(t *testing.T) {
		// A helper that recovers a runtime error reports the runtime's own
		// line, as go test does for an index out of range.
		h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
			return []runnerapi.Event{
				action("run", "TestParse"),
				action("run", "TestParse/out"),
				output("TestParse/out", "=== RUN   TestParse/out\n"),
				output("TestParse/out", "    panic.go:115: panicked: runtime error: index out of range [3] with length 1\n"),
				output("TestParse/out", "--- FAIL: TestParse/out (0.00s)\n"),
				action("fail", "TestParse/out"),
				output("TestParse", "--- FAIL: TestParse (0.00s)\n"),
				action("fail", "TestParse"),
				{Type: runnerapi.EventDone, ExitCode: 1, Failed: 1},
			}
		}
		h.clock.Advance(time.Minute)
		_, explains, _ := h.runTest(admin, map[string]any{})
		if len(explains) != 1 || explains[0]["test"] != "TestParse/out" {
			t.Fatalf("explain events = %v, want 1 for TestParse/out", explains)
		}
		raw := items(explains[0])[0]["raw"].(map[string]any)
		if raw["file"] != "panic.go" || raw["line"] != 115.0 || raw["openable"] != nil {
			t.Errorf("raw = %v, want panic.go:115 and not openable", raw)
		}
	})

	t.Run("go test's timeout", func(t *testing.T) {
		// When -timeout fires, no test reports a result; the panic and the
		// goroutines are in the output of the test that was running.
		h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
			sub := "TestParseBareValues/single_pair"
			return []runnerapi.Event{
				action("run", "TestParseBareValues"),
				output("TestParseBareValues", "=== RUN   TestParseBareValues\n"),
				action("run", sub),
				output(sub, "=== RUN   "+sub+"\n"),
				output(sub, "panic: test timed out after 35s\n"),
				output(sub, "\trunning tests:\n"),
				output(sub, "\t\tTestParseBareValues (35s)\n"),
				output(sub, "\t\t"+sub+" (35s)\n"),
				output(sub, "\n"),
				// The parent waits in t.Run of the course's table helper;
				// the explanation points past it, at the stub.
				output(sub, "goroutine 6 [chan receive]:\n"),
				output(sub, "testing.(*T).Run(0xc00012c1c0, {0x720084, 0xb}, 0xc00006cbc0)\n"),
				output(sub, "\t/usr/local/go/src/testing/testing.go:1859 +0x91e\n"),
				output(sub, "github.com/yornik/go-practice/exercises/01-logfmt_test.runParseCases(0xc00012c1c0, {0xc000135a78, 0xb, 0xc000057850?})\n"),
				output(sub, "\texercises/01-logfmt/logfmt_test.go:37 +0xa5\n"),
				output(sub, "\n"),
				output(sub, "goroutine 7 [select (no cases)]:\n"),
				output(sub, "github.com/yornik/go-practice/exercises/01-logfmt.Parse({0x5a1b2c, 0x3})\n"),
				output(sub, "\texercises/01-logfmt/logfmt.go:45 +0x18\n"),
				output("", "FAIL\tgithub.com/yornik/go-practice/exercises/01-logfmt\t35.012s\n"),
				action("fail", ""),
				{Type: runnerapi.EventDone, ExitCode: 1, TimedOut: true},
			}
		}
		h.clock.Advance(time.Minute)
		last, explains, types := h.runTest(admin, map[string]any{})
		if len(explains) != 1 || explains[0]["test"] != "TestParseBareValues/single_pair" {
			t.Fatalf("explain events = %v, want 1 for the running subtest", explains)
		}
		it := items(explains[0])[0]
		raw := it["raw"].(map[string]any)
		if it["id"] != "test-timeout" || raw["file"] != "logfmt.go" || raw["line"] != 45.0 || raw["openable"] != true {
			t.Errorf("explanation = %v, want test-timeout at logfmt.go:45", it)
		}
		// The explanation comes before done, and the run passes nothing.
		if n := len(types); n < 3 || types[n-3] != "explain" || types[n-2] != "done" || types[n-1] != "progress" {
			t.Errorf("event types end with %v, want explain done progress", types)
		}
		if last["progress"]["passed"] != false {
			t.Errorf("progress = %v", last["progress"])
		}
	})

	// A subtest whose code exits the test binary. Its parent logged a
	// got/want first and then waited in t.Run.
	crash := func(done runnerapi.Event) func(runnerapi.TestRequest) []runnerapi.Event {
		return func(runnerapi.TestRequest) []runnerapi.Event {
			sub := "TestParse/exit"
			return []runnerapi.Event{
				action("run", "TestParse"),
				output("TestParse", "=== RUN   TestParse\n"),
				output("TestParse", "    logfmt_test.go:7: Parse(\"a\") = 1, want 2\n"),
				action("run", sub),
				output(sub, "=== RUN   "+sub+"\n"),
				output(sub, "    logfmt_test.go:9: Parse(\"b\") = 1, want 3\n"),
				output("", "FAIL\tgithub.com/yornik/go-practice/exercises/01-logfmt\t0.012s\n"),
				action("fail", ""),
				done,
			}
		}
	}

	t.Run("a crashed subtest and its parent", func(t *testing.T) {
		h.runner.events = crash(runnerapi.Event{Type: runnerapi.EventDone, ExitCode: 1})
		h.clock.Advance(time.Minute)
		_, explains, _ := h.runTest(admin, map[string]any{})
		ids := map[string][]any{}
		for _, ev := range explains {
			for _, it := range items(ev) {
				ids[ev["test"].(string)] = append(ids[ev["test"].(string)], it["id"])
			}
		}
		if got := fmt.Sprint(ids["TestParse/exit"]); !strings.Contains(got, "runtime-exit") {
			t.Errorf("TestParse/exit explanations = %v, want the exit", got)
		}
		if got := fmt.Sprint(ids["TestParse"]); got != "[test-got-want]" {
			t.Errorf("TestParse explanations = %v, want only its own got/want", got)
		}
	})

	for _, c := range []struct {
		name string
		done runnerapi.Event
	}{
		{"cut-off output", runnerapi.Event{Type: runnerapi.EventDone, ExitCode: -1, Truncated: true}},
		{"a stopped run", runnerapi.Event{Type: runnerapi.EventDone, ExitCode: -1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The output ends early, so an explanation of the unfinished
			// tests would guess.
			h.runner.events = crash(c.done)
			h.clock.Advance(time.Minute)
			_, explains, _ := h.runTest(admin, map[string]any{})
			if len(explains) != 0 {
				t.Errorf("explain events = %v, want none", explains)
			}
		})
	}
}
