package runner

import (
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// TestEveryExercise runs the whole course through the sandbox, so a course
// bump proves that the runner serves every exercise: each reference
// solution passes with -race, each unmodified stub fails without a build
// error, a timeout or cut-off output, and the check of each stub and each
// solution reports nothing. A new exercise that needs something the sandbox
// denies (a file, a port, a signal) fails here, not in a learner's browser.
// It takes about a minute; -short skips it with the other tests that run the
// toolchain.
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
			// A cut-off first run hides the summary a learner starts from.
			if done.ExitCode == 0 || done.Failed == 0 || done.BuildFailed || done.TimedOut || done.Truncated {
				t.Errorf("stub: done = %+v, want failed tests that build, end in time and fit the output cap", done)
			}
			if !hasOutput(events, "TODO: implement") {
				t.Error("stub: no output with the stub's panic")
			}

			// A learner who compares with the reference solution in the
			// IDE should see no warnings on the model answer either.
			for name, files := range map[string]map[string]string{"stub": stub, "solution": solutionFiles(t, ex)} {
				var check runnerapi.CheckResponse
				env.postJSON(t, runnerapi.PathCheck, runnerapi.CheckRequest{Exercise: ex, Files: files}, &check, 200)
				for _, d := range check.Diagnostics {
					t.Errorf("%s check: %s:%d:%d %s %s %s", name, d.File, d.Line, d.Column, d.Source, d.Code, d.Message)
				}
			}
		})
	}
}
