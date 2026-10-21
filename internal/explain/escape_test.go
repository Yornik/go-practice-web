package explain

import (
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// TestLearnerOutputIsEscaped feeds markup through every path that captures
// text from tool or test output into an explanation. Learner code controls
// that text (panic values, test names, t.Errorf messages, identifiers),
// and the IDE inserts the HTML as is.
func TestLearnerOutputIsEscaped(t *testing.T) {
	const evil = `<img src=x onerror=alert(1)>`
	outputs := map[string]string{
		"panic": "--- FAIL: TestX (0.00s)\npanic: " + evil + " [recovered]\n\tpanic: " + evil + "\n\ngoroutine 7 [running]:\n" +
			"example.com/x." + evil + "(...)\n\texercises/26-logfmt/logfmt.go:30 +0x25\n",
		"assertion": "    logfmt_test.go:42: Parse(" + evil + ") = " + evil + ", want " + evil + "\n--- FAIL: TestX (0.00s)\n",
		"got want":  "    logfmt_test.go:42: got " + evil + "\n        want " + evil + "\n--- FAIL: TestX (0.00s)\n",
		"race": "==================\nWARNING: DATA RACE\nWrite at 0x00c000 by goroutine 8:\n  example.com/x." + evil + "()\n" +
			"      /ws/exercises/37-workerpool/pool.go:12 +0x44\n\nPrevious read at 0x00c000 by goroutine 7:\n  example.com/x." + evil + "()\n" +
			"      /ws/exercises/37-workerpool/pool.go:20 +0x44\n==================\n    testing.go:1490: race detected during execution of test\n--- FAIL: TestX (0.00s)\n",
		"timeout":  "panic: test timed out after 2s\n\trunning tests:\n\t\t" + evil + " (2s)\n",
		"deadlock": "fatal error: all goroutines are asleep - deadlock!\n\ngoroutine 1 [chan receive]:\nexample.com/x." + evil + "()\n",
		"example":  "--- FAIL: Example" + evil + " (0.00s)\ngot:\n" + evil + "\nwant:\n" + evil + "\n",
		"fatal":    "fatal error: " + evil + "\n",
	}
	reached := false // the markup made it into some explanation, escaped
	for name, out := range outputs {
		for _, test := range []string{"TestX", "TestX/" + evil, ""} {
			for _, e := range ExplainTestOutput(test, out) {
				if strings.Contains(e.HTML, "<img") {
					t.Errorf("%s: %s (test %q): HTML holds the markup unescaped:\n%s", name, e.ID, test, e.HTML)
				}
				reached = reached || strings.Contains(e.HTML, "&lt;img")
			}
		}
	}
	if !reached {
		t.Error("no explanation captured the markup; the test checks nothing")
	}
	build := "# example.com/x\nexercises/26-logfmt/logfmt.go:3:2: undefined: " + evil + "\nexercises/26-logfmt/logfmt.go:4:2: " + evil + "\n"
	diags, exps := ExplainBuildOutput(build)
	if len(diags) == 0 {
		t.Fatal("no diagnostics from the build output")
	}
	for _, e := range exps {
		if strings.Contains(e.HTML, "<img") {
			t.Errorf("%s: build explanation holds the markup unescaped:\n%s", e.ID, e.HTML)
		}
	}
	for _, src := range []string{"compile", "vet", "staticcheck", "gofmt"} {
		e := Explain(runnerapi.Diagnostic{File: evil + ".go", Line: 1, Source: src, Code: evil, Message: evil})
		if strings.Contains(e.HTML, "<img") {
			t.Errorf("%s diagnostic: HTML holds the markup unescaped:\n%s", src, e.HTML)
		}
	}
}
