package runner

import (
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// TestEveryExercise runs the whole course through the sandbox, so a course
// bump proves that the runner serves every exercise: each reference
// solution passes with -race, each unmodified stub fails without a build
// error or a timeout, and the check of each stub reports nothing. A new
// exercise that needs something the sandbox denies (a file, a port, a
// signal) fails here, not in a learner's browser. It takes about a minute;
// -short skips it with the other tests that run the toolchain.
func TestEveryExercise(t *testing.T) {
	slow(t)
	t.Parallel()
	// No queue bound and room for every subtest: they wait for the 4
	// slots instead of ending busy.
	env := newTestEnv(t, func(o *Options) { o.MaxConcurrent, o.MaxQueue, o.MaxQueueWait = 4, 128, 0 })
	t.Cleanup(func() { assertWorkDirEmpty(t, env.workDir) })
	exercises := env.srv.opts.Course.Exercises()
	if len(exercises) < 50 {
		t.Fatalf("the course has %d exercises", len(exercises))
	}
	for _, ex := range exercises {
		t.Run(ex, func(t *testing.T) {
			t.Parallel()
			done := doneEvent(t, env.runTests(t, runnerapi.TestRequest{Exercise: ex, Files: solutionFiles(t, ex), Race: true}))
			if done.ExitCode != 0 || done.Failed != 0 || done.Passed == 0 || done.TimedOut || done.Truncated || done.RaceUnavailable {
				t.Errorf("solution: done = %+v, want a clean pass with -race", done)
			}

			stub := stubFiles(t, ex)
			events := env.runTests(t, runnerapi.TestRequest{Exercise: ex, Files: stub, Race: true})
			done = doneEvent(t, events)
			// A stub may hit the output cap: 29-grep's stub fails 200 cases,
			// each with the panic's stack.
			if done.ExitCode == 0 || done.Failed == 0 || done.BuildFailed || done.TimedOut {
				t.Errorf("stub: done = %+v, want failed tests that build and end in time", done)
			}
			if !hasOutput(events, "TODO: implement") {
				t.Error("stub: no output with the stub's panic")
			}

			var check runnerapi.CheckResponse
			env.postJSON(t, runnerapi.PathCheck, runnerapi.CheckRequest{Exercise: ex, Files: stub}, &check, 200)
			for _, d := range check.Diagnostics {
				t.Errorf("stub check: %s:%d:%d %s %s %s", d.File, d.Line, d.Column, d.Source, d.Code, d.Message)
			}
		})
	}
}
