package explain

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func readOutput(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "output", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// guideFile is a minimal valid guide for test catalogs.
func guideFile(id string) string {
	return "---\nid: " + id + "\nsource: guide\ntitle: Guide " + id +
		"\nlinks:\n- Go | https://go.dev/doc/\n---\n## Read it\n\nText.\n\n## Learn more\n"
}

// entryFile is a valid entry with the 5 sections and the given front matter
// lines after id and source.
func entryFile(id, source, front, body string) string {
	if body == "" {
		body = "Text."
	}
	return "---\nid: " + id + "\nsource: " + source + "\n" + front +
		"links:\n- Go | https://go.dev/doc/\n---\n## What it means\n\n" + body +
		"\n\n## Why Go does this\n\nText.\n\n## How to find it\n\nText.\n\n## Example\n\nText.\n\n## Learn more\n"
}

// testCatalog loads a catalog with the 4 guides and the given entries,
// keyed by "source/id".
func testCatalog(t *testing.T, entries map[string]string) *catalogData {
	t.Helper()
	fsys := fstest.MapFS{}
	for _, id := range []string{GuideErrorMessage, GuideStackTrace, GuideRaceReport, GuideFailingTest} {
		fsys["catalog/guide/"+id+".md"] = &fstest.MapFile{Data: []byte(guideFile(id))}
	}
	for key, src := range entries {
		fsys["catalog/"+key+".md"] = &fstest.MapFile{Data: []byte(src)}
	}
	c, err := loadCatalog(fsys)
	if err != nil {
		t.Fatalf("test catalog: %v", err)
	}
	return c
}

// explainAll runs the scanner on output against catalog c.
func explainAll(c *catalogData, test, output string) []Explanation {
	var out []Explanation
	for _, f := range scanTestOutput(test, normalizeNewlines(output)) {
		out = append(out, c.explain(f))
	}
	return out
}

func TestCatalogLoads(t *testing.T) {
	if err := Check(); err != nil {
		t.Fatalf("catalog:\n%v", err)
	}
}

func TestExplainDiagnostic(t *testing.T) {
	tests := []struct {
		name      string
		d         runnerapi.Diagnostic
		id        string
		title     string
		captures  map[string]string
		generic   bool
		inHTML    []string
		notInHTML []string
	}{
		{
			name:     "undefined",
			d:        runnerapi.Diagnostic{File: "retry.go", Line: 12, Column: 5, Source: "compile", Message: "undefined: backof"},
			id:       "compile-undefined",
			title:    "backof is not declared where you use it",
			captures: map[string]string{"name": "backof", "file": "retry.go", "line": "12"},
			inHTML:   []string{"<code>backof</code>", "retry.go:12", `href="https://go.dev/ref/spec#Declarations_and_scope"`},
		},
		{
			name:     "undefined with a suggestion",
			d:        runnerapi.Diagnostic{File: "x.go", Line: 3, Source: "compile", Message: "undefined: strings.contains (but have Contains)"},
			id:       "compile-undefined",
			captures: map[string]string{"name": "strings.contains", "have": "Contains"},
		},
		{
			name:   "gofmt syntax errors are compile errors",
			d:      runnerapi.Diagnostic{File: "x.go", Line: 3, Source: "gofmt", Message: "undefined: x"},
			id:     "compile-undefined",
			inHTML: []string{"<code>x</code>"},
		},
		{
			name:      "captured values are escaped",
			d:         runnerapi.Diagnostic{File: "x.go", Line: 3, Source: "compile", Message: "undefined: a<b>&c"},
			id:        "compile-undefined",
			title:     "a<b>&c is not declared where you use it",
			inHTML:    []string{"<code>a&lt;b&gt;&amp;c</code>"},
			notInHTML: []string{"a<b>"},
		},
		{
			name:     "declared and not used",
			d:        runnerapi.Diagnostic{File: "x.go", Line: 6, Column: 2, Source: "compile", Message: "declared and not used: unused"},
			id:       "compile-declared-and-not-used",
			captures: map[string]string{"name": "unused"},
		},
		{
			name: "vet printf with the analyzer",
			d: runnerapi.Diagnostic{File: "x.go", Line: 8, Column: 14, Source: "vet", Code: "printf",
				Message: "fmt.Printf format %d has arg s of wrong type string"},
			id:       "vet-printf-wrong-type",
			title:    "The verb %d in fmt.Printf does not fit a string",
			captures: map[string]string{"call": "fmt.Printf", "verb": "%d", "arg": "s", "type": "string", "code": "printf"},
		},
		{
			name: "vet printf without the analyzer, as go test prints it",
			d: runnerapi.Diagnostic{File: "x.go", Line: 8, Column: 14, Source: "vet",
				Message: "fmt.Sprintf format %s has arg n of wrong type int"},
			id: "vet-printf-wrong-type",
		},
		{
			name:     "staticcheck with the code",
			d:        runnerapi.Diagnostic{File: "x.go", Line: 9, Column: 2, Source: "staticcheck", Code: "SA4006", Message: "this value of err is never used"},
			id:       "staticcheck-sa4006",
			title:    "The value stored in err is never read",
			captures: map[string]string{"name": "err", "code": "SA4006"},
		},
		{
			name:     "staticcheck code left in the message",
			d:        runnerapi.Diagnostic{File: "x.go", Line: 9, Source: "staticcheck", Message: "this value of n is never used (SA4006)"},
			id:       "staticcheck-sa4006",
			captures: map[string]string{"name": "n", "code": "SA4006"},
		},
		{
			name: "staticcheck compile code",
			d:    runnerapi.Diagnostic{File: "x.go", Line: 9, Source: "staticcheck", Code: "compile", Message: "undefined: total"},
			id:   "compile-undefined",
		},
		{
			name:    "unknown compile error",
			d:       runnerapi.Diagnostic{File: "x.go", Line: 1, Source: "compile", Message: "something new in Go 9"},
			id:      GuideErrorMessage,
			title:   "How to read a Go error message",
			generic: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Explain(tc.d)
			if e.ID != tc.id {
				t.Fatalf("ID = %q, want %q (title %q)", e.ID, tc.id, e.Title)
			}
			if tc.title != "" && e.Title != tc.title {
				t.Errorf("Title = %q, want %q", e.Title, tc.title)
			}
			if e.Generic != tc.generic {
				t.Errorf("Generic = %v, want %v", e.Generic, tc.generic)
			}
			for k, v := range tc.captures {
				if got := e.Raw.Captures[k]; got != v {
					t.Errorf("Raw.Captures[%q] = %q, want %q (all: %v)", k, got, v, e.Raw.Captures)
				}
			}
			for _, s := range tc.inHTML {
				if !strings.Contains(e.HTML, s) {
					t.Errorf("HTML does not contain %q:\n%s", s, e.HTML)
				}
			}
			for _, s := range tc.notInHTML {
				if strings.Contains(e.HTML, s) {
					t.Errorf("HTML contains %q:\n%s", s, e.HTML)
				}
			}
			if strings.Contains(e.HTML, "{{") || strings.Contains(e.Title, "{{") {
				t.Errorf("unfilled placeholder:\n%s\n%s", e.Title, e.HTML)
			}
			if e.Raw.File != tc.d.File || e.Raw.Line != tc.d.Line || e.Raw.Column != tc.d.Column {
				t.Errorf("Raw position = %s:%d:%d, want %s:%d:%d", e.Raw.File, e.Raw.Line, e.Raw.Column, tc.d.File, tc.d.Line, tc.d.Column)
			}
			if len(e.Links) == 0 {
				t.Errorf("no links")
			}
		})
	}
}

func TestExplanationJSON(t *testing.T) {
	e := Explain(runnerapi.Diagnostic{File: "x.go", Line: 1, Source: "compile", Message: "undefined: x"})
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "source", "title", "html", "links", "raw"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON has no %q: %s", key, b)
		}
	}
	links, _ := m["links"].([]any)
	if len(links) == 0 {
		t.Fatalf("links = %v", m["links"])
	}
	if l, _ := links[0].(map[string]any); l["label"] == nil || l["url"] == nil {
		t.Errorf("link = %v, want label and url", links[0])
	}
}

func TestStubPanic(t *testing.T) {
	out := readOutput(t, "stub-panic.txt")
	exps := ExplainTestOutput("", out)
	if len(exps) == 0 || exps[0].ID != "runtime-todo-implement" {
		t.Fatalf("got %v, want runtime-todo-implement first", ids(exps))
	}
	e := exps[0]
	if e.Title != "Parse is still the course stub" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Raw.File != "logfmt.go" || e.Raw.Line != 45 || e.Raw.Message != "panic: TODO: implement" {
		t.Errorf("Raw = %+v", e.Raw)
	}
	if e.Raw.Test != "TestParseQuotedValues/spaces_inside_quotes" {
		t.Errorf("Raw.Test = %q", e.Raw.Test)
	}
	// The test name from the caller wins.
	if got := ExplainTestOutput("TestParseQuotedValues", out)[0].Raw.Test; got != "TestParseQuotedValues" {
		t.Errorf("Raw.Test = %q, want the caller's name", got)
	}
}

func TestRaceReport(t *testing.T) {
	exps := ExplainTestOutput("TestRace", readOutput(t, "race.txt"))
	if len(exps) < 2 || exps[0].ID != "race-data-race" {
		t.Fatalf("got %v, want race-data-race and then the assertion", ids(exps))
	}
	e := exps[0]
	want := map[string]string{
		"func": "Count.func1", "file": "y.go", "line": "12", "access": "read",
		"prev_func": "Count.func1", "prev_file": "y.go", "prev_line": "12", "prev_access": "write",
	}
	for k, v := range want {
		if got := e.Raw.Captures[k]; got != v {
			t.Errorf("Captures[%q] = %q, want %q", k, got, v)
		}
	}
	if e.Course != "20-counters" || !strings.Contains(e.HTML, `href="/learn/20-counters"`) {
		t.Errorf("Course = %q; HTML has no course link", e.Course)
	}
	if !strings.HasPrefix(e.Raw.Message, "WARNING: DATA RACE\nRead at") || strings.Contains(e.Raw.Message, "created at") {
		t.Errorf("Raw.Message = %q, want the headline of the report", e.Raw.Message)
	}
	// Only the first of the 2 reports is explained.
	n := 0
	for _, x := range exps {
		if x.ID == "race-data-race" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d race explanations, want 1", n)
	}
	a := exps[1]
	if a.Source != SourceGuide && a.Source != SourceTest || a.Raw.File != "y_test.go" || a.Raw.Line != 13 {
		t.Errorf("second = %s %+v, want the assertion at y_test.go:13", a.ID, a.Raw)
	}
}

func TestHangingGoroutines(t *testing.T) {
	const (
		alarm = "goroutine 18 [running]:\ntesting.(*M).startAlarm.func1()\n\t/usr/local/go/src/testing/testing.go:2484 +0x605\n" +
			"created by time.goFunc\n\t/usr/local/go/src/time/sleep.go:215 +0x45\n\n"
		parent = "goroutine 6 [chan receive]:\ntesting.(*T).Run(0xc00012c1c0, {0x720084, 0xb}, 0xc00006cbc0)\n" +
			"\t/usr/local/go/src/testing/testing.go:1859 +0x91e\n" +
			"example.com/x_test.TestDo(0xc00012c1c0)\n\t/w/x_test.go:37 +0xa5\n" +
			"testing.tRunner(0xc00012c1c0, 0x7308b0)\n\t/usr/local/go/src/testing/testing.go:1792 +0x226\n\n"
		leaked = "goroutine 7 [chan receive]:\nexample.com/x.Watch(...)\n\t/w/x.go:22\n" +
			"created by example.com/x_test.TestWatch in goroutine 5\n\t/w/x_test.go:10 +0xdb\n\n"
		hanging = "goroutine 9 [chan send]:\nexample.com/x.Do(...)\n\t/w/x.go:46\n" +
			"example.com/x_test.TestDo.func1(0xc00012c540)\n\t/w/x_test.go:38 +0x65\n" +
			"testing.tRunner(0xc00012c540, 0xc00006cbc0)\n\t/usr/local/go/src/testing/testing.go:1792 +0x226\n\n"
		stdOnly = "goroutine 8 [IO wait]:\ninternal/poll.runtime_pollWait(0x7f, 0x72)\n\t/usr/local/go/src/runtime/netpoll.go:351 +0x85\n\n"
	)
	tests := []struct {
		name string
		dump string
		file string
		line int
	}{
		{"the test that hangs, past its parent and a leaked goroutine", alarm + parent + leaked + hanging, "x.go", 46},
		{"a goroutine the code started", alarm + parent + leaked, "x.go", 22},
		{"no goroutine with exercise code: the whole dump", alarm + parent + stdOnly, "x_test.go", 37},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr, ok := firstUserFrame(parseFrames(hangingGoroutines(strings.Split(tt.dump, "\n"))))
			if !ok || path.Base(fr.path) != tt.file || fr.line != tt.line {
				t.Errorf("frame = %+v (found %v), want %s:%d", fr, ok, tt.file, tt.line)
			}
		})
	}
}

func TestScanTestOutput(t *testing.T) {
	c := testCatalog(t, nil)
	tests := []struct {
		file    string
		test    string
		source  string
		guide   string
		message string
		rawFile string
		rawLine int
		rawTest string
		fn      string
	}{
		{"deadlock.txt", "", SourceRuntime, GuideStackTrace, "fatal error: all goroutines are asleep - deadlock!", "y.go", 25, "", "Stuck"},
		{"timeout.txt", "", SourceTest, GuideStackTrace, "panic: test timed out after 1s", "y_test.go", 35, "TestSlow", ""},
		// The runner's output of a table test whose subtest hangs: the
		// parent waits in t.Run at logfmt_test.go:37, the subtest at the
		// channel send in the stub.
		{"timeout-subtest.txt", "TestParseBareValues/single_pair", SourceTest, GuideStackTrace, "panic: test timed out after 10s", "logfmt.go", 46, "TestParseBareValues/single_pair", ""},
		{"nil.txt", "", SourceRuntime, GuideStackTrace, "panic: runtime error: invalid memory address or nil pointer dereference", "y.go", 32, "TestNil", "Nil"},
		{"example.txt", "", SourceTest, GuideFailingTest, "got:\n1\nhello\nwant:\n1\nworld", "", 0, "ExampleCount", ""},
		{"gotwant.txt", "", SourceTest, GuideFailingTest, "Add(1, 2) = 3, want 4", "y_test.go", 29, "TestGotWant/case_one", ""},
		{"gotwant-v.txt", "", SourceTest, GuideFailingTest, "Add(1, 2) = 3, want 4", "y_test.go", 29, "TestGotWant/case_one", ""},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			fs := scanTestOutput(tc.test, readOutput(t, tc.file))
			if len(fs) == 0 {
				t.Fatal("no findings")
			}
			f := fs[0]
			if f.source != tc.source || f.guide != tc.guide {
				t.Errorf("source, guide = %s, %s; want %s, %s", f.source, f.guide, tc.source, tc.guide)
			}
			if f.raw.Message != tc.message {
				t.Errorf("message = %q, want %q", f.raw.Message, tc.message)
			}
			if f.raw.File != tc.rawFile || f.raw.Line != tc.rawLine || f.raw.Test != tc.rawTest {
				t.Errorf("raw = %s:%d test %q, want %s:%d test %q", f.raw.File, f.raw.Line, f.raw.Test, tc.rawFile, tc.rawLine, tc.rawTest)
			}
			if got := f.builtins["func"]; got != tc.fn {
				t.Errorf("func = %q, want %q", got, tc.fn)
			}
			e := c.explain(f)
			if e.ID != tc.guide || !e.Generic {
				t.Errorf("explanation %s (generic %v), want the guide %s", e.ID, e.Generic, tc.guide)
			}
		})
	}
}

func TestAssertions(t *testing.T) {
	fs := scanTestOutput("", readOutput(t, "gotwant.txt"))
	if len(fs) != 2 {
		t.Fatalf("%d findings, want 2", len(fs))
	}
	second := fs[1].raw
	if second.Test != "TestGotWant" || second.Line != 32 || second.Message != "Parse(\"a\")\ngot  [\"x\"]\nwant [\"y\"]" {
		t.Errorf("second = %+v", second)
	}
	// 2 assertions explained by the same entry give 1 explanation.
	if exps := explainAll(testCatalog(t, nil), "", readOutput(t, "gotwant.txt")); len(exps) != 2 {
		t.Fatalf("explainAll: %d", len(exps))
	}
	if exps := ExplainTestOutput("", readOutput(t, "gotwant.txt")); len(exps) != 1 {
		t.Errorf("ExplainTestOutput gave %v, want 1 explanation", ids(exps))
	}
}

func TestTestOutputPatterns(t *testing.T) {
	c := testCatalog(t, map[string]string{
		"test/test-timeout": entryFile("test-timeout", "test",
			"match: ^panic: test timed out after (?P<after>\\S+)$\ntitle: {{test}} ran longer than {{after}}\n", ""),
		"test/test-example": entryFile("test-example", "test",
			"match: (?s)^got:\\n(?P<got>.*)\\nwant:\\n(?P<want>.*)$\ntitle: {{test}} printed something else\n", "Got {{got}}."),
		"runtime/runtime-deadlock": entryFile("runtime-deadlock", "runtime",
			"match: ^fatal error: all goroutines are asleep - deadlock!$\ntitle: Deadlock in {{func}}\n", ""),
	})
	tests := []struct{ file, id, title string }{
		{"timeout.txt", "test-timeout", "TestSlow ran longer than 1s"},
		{"example.txt", "test-example", "ExampleCount printed something else"},
		{"deadlock.txt", "runtime-deadlock", "Deadlock in Stuck"},
	}
	for _, tc := range tests {
		exps := explainAll(c, "", readOutput(t, tc.file))
		if len(exps) == 0 || exps[0].ID != tc.id || exps[0].Title != tc.title {
			t.Errorf("%s: got %v, want %s %q", tc.file, exps, tc.id, tc.title)
		}
	}
	if exps := explainAll(c, "", readOutput(t, "example.txt")); !strings.Contains(exps[0].HTML, "Got 1\nhello.") {
		t.Errorf("multi-line capture: %s", exps[0].HTML)
	}
}

func TestBuildFailureInTestOutput(t *testing.T) {
	exps := ExplainTestOutput("", readOutput(t, "build-vet.txt"))
	if len(exps) != 1 || exps[0].ID != "vet-printf-wrong-type" {
		t.Fatalf("vet: got %v", ids(exps))
	}
	if exps[0].Raw.Source != SourceVet || exps[0].Raw.File != "z.go" || exps[0].Raw.Line != 6 || exps[0].Raw.Column != 22 {
		t.Errorf("vet Raw = %+v", exps[0].Raw)
	}
	exps = ExplainTestOutput("", readOutput(t, "build-compile.txt"))
	if len(exps) != 3 || exps[0].ID != "compile-declared-and-not-used" || exps[1].ID != "compile-undefined" {
		t.Fatalf("compile: got %v", ids(exps))
	}
	if r := exps[2].Raw; r.File != "z_test.go" || r.Line != 5 || r.Column != 32 || !strings.HasPrefix(r.Message, "assignment mismatch") {
		t.Errorf("compile: third Raw = %+v", r)
	}
}

func TestMatchRanks(t *testing.T) {
	c := testCatalog(t, map[string]string{
		"staticcheck/staticcheck-sa4":        entryFile("staticcheck-sa4", "staticcheck", "code: SA4\ntitle: SA4 {{code}}\n", ""),
		"staticcheck/staticcheck-sa4006":     entryFile("staticcheck-sa4006", "staticcheck", "code: SA4006\ntitle: SA4006\n", ""),
		"staticcheck/staticcheck-sa4006-err": entryFile("staticcheck-sa4006-err", "staticcheck", "code: SA4006\nmatch: ^this value of err is never used$\ntitle: err\n", ""),
		"staticcheck/staticcheck-sa4-pat":    entryFile("staticcheck-sa4-pat", "staticcheck", "code: SA4\nmatch: ^special$\ntitle: special\n", ""),
		"vet/vet-printf":                     entryFile("vet-printf", "vet", "analyzer: printf\ntitle: printf\n", ""),
		"vet/vet-printf-verb":                entryFile("vet-printf-verb", "vet", "analyzer: printf\nmatch: ^(?P<call>\\S+) format\ntitle: {{call}}\n", ""),
		"vet/vet-anything-loud":              entryFile("vet-anything-loud", "vet", "match: LOUD\ntitle: loud\n", ""),
		"build/build-gomod":                  entryFile("build-gomod", "build", "match: ^go: updates to go.mod needed\ntitle: go.mod\n", ""),
	})
	tests := []struct {
		source, code, msg, id string
	}{
		{"staticcheck", "SA4006", "this value of err is never used", "staticcheck-sa4006-err"},
		{"staticcheck", "SA4006", "this value of n is never used", "staticcheck-sa4006"},
		{"staticcheck", "SA4017", "something", "staticcheck-sa4"},
		{"staticcheck", "SA4017", "special", "staticcheck-sa4-pat"},
		{"staticcheck", "SA40", "something", "staticcheck-sa4"},
		{"staticcheck", "S1000", "something", GuideErrorMessage},
		{"staticcheck", "", "this value of err is never used", "staticcheck-sa4006-err"},
		{"staticcheck", "", "this value of n is never used", GuideErrorMessage},
		{"vet", "printf", "fmt.Printf format %d has arg", "vet-printf-verb"},
		{"vet", "printf", "fmt.Println call has possible Printf formatting directive %d", "vet-printf"},
		{"vet", "copylocks", "fmt.Printf format %d has arg", GuideErrorMessage},
		{"vet", "copylocks", "LOUD", "vet-anything-loud"},
		{"vet", "", "fmt.Printf format %d has arg", "vet-printf-verb"},
		{"vet", "", "fmt.Println call has possible Printf formatting directive %d", GuideErrorMessage},
		{"compile", "", "go: updates to go.mod needed; to update it:\n\tgo mod tidy", "build-gomod"},
	}
	for _, tc := range tests {
		f := finding{source: tc.source, code: tc.code, guide: GuideErrorMessage, raw: Raw{Message: tc.msg}}
		if e := c.explain(f); e.ID != tc.id {
			t.Errorf("%s %s %q: got %s, want %s", tc.source, tc.code, tc.msg, e.ID, tc.id)
		}
	}
}

func TestExplainBuildOutput(t *testing.T) {
	out := "# example.com/x [example.com/x.test]\n" +
		"./x.go:4:2: declared and not used: x\n" +
		"./x.go:7:9: too many return values\n\thave (number)\n\twant ()\n" +
		"x.go:9: undefined: y\n" +
		"vet: ./x.go:10:1: undefined: z\n" +
		"./x.go:11:2: too many errors\n" +
		"# [example.com/x]\n" +
		"./x.go:12:2: fmt.Printf format %d has arg s of wrong type string\n" +
		"go: updates to go.mod needed; to update it:\n\tgo mod tidy\n" +
		"FAIL\texample.com/x [build failed]\n"
	diags, exps := ExplainBuildOutput(out)
	want := []runnerapi.Diagnostic{
		{File: "x.go", Line: 4, Column: 2, Severity: "error", Source: "compile", Message: "declared and not used: x"},
		{File: "x.go", Line: 7, Column: 9, Severity: "error", Source: "compile", Message: "too many return values\n\thave (number)\n\twant ()"},
		{File: "x.go", Line: 9, Severity: "error", Source: "compile", Message: "undefined: y"},
		{File: "x.go", Line: 10, Column: 1, Severity: "error", Source: "compile", Message: "undefined: z"},
		{File: "x.go", Line: 12, Column: 2, Severity: "error", Source: "vet", Message: "fmt.Printf format %d has arg s of wrong type string"},
		{Severity: "error", Source: "compile", Message: "go: updates to go.mod needed; to update it:\n\tgo mod tidy"},
	}
	if len(diags) != len(want) || len(exps) != len(diags) {
		t.Fatalf("got %d diagnostics and %d explanations, want %d:\n%+v", len(diags), len(exps), len(want), diags)
	}
	for i := range want {
		if diags[i] != want[i] {
			t.Errorf("diagnostic %d = %+v, want %+v", i, diags[i], want[i])
		}
	}
	wantIDs := []string{"compile-declared-and-not-used", "", "compile-undefined", "compile-undefined", "vet-printf-wrong-type", ""}
	for i, id := range wantIDs {
		if id != "" && exps[i].ID != id {
			t.Errorf("explanation %d = %s, want %s", i, exps[i].ID, id)
		}
	}
}

func TestRefs(t *testing.T) {
	out := readOutput(t, "stub-panic.txt") + readOutput(t, "race.txt") + "./z.go:4:2: declared and not used: x\n"
	refs := Refs(out)
	type key struct {
		file string
		line int
		col  int
		test bool
	}
	var got []key
	for _, r := range refs {
		got = append(got, key{r.File, r.Line, r.Column, r.Test})
		text := out[r.Start:r.End]
		if !strings.HasSuffix(text, r.File+":"+itoa(r.Line)+map[bool]string{true: ":" + itoa(r.Column), false: ""}[r.Column != 0]) {
			t.Errorf("out[%d:%d] = %q does not end with the reference %s:%d", r.Start, r.End, text, r.File, r.Line)
		}
	}
	first := []key{{"logfmt.go", 45, 0, false}, {"logfmt_test.go", 38, 0, true}}
	if len(got) < 2 || got[0] != first[0] || got[1] != first[1] {
		t.Errorf("first refs = %v, want %v", got, first)
	}
	if last := got[len(got)-1]; last != (key{"z.go", 4, 2, false}) {
		t.Errorf("last ref = %v", last)
	}
	for _, r := range refs {
		if r.File == "testing.go" || r.File == "_testmain.go" || r.File == "panic.go" {
			t.Errorf("ref into the standard library: %+v", r)
		}
	}
	var sawAssert bool
	for _, k := range got {
		if k == (key{"y_test.go", 13, 0, true}) {
			sawAssert = true
		}
	}
	if !sawAssert {
		t.Errorf("no ref for the assertion y_test.go:13 in %v", got)
	}
}

func TestShortFunc(t *testing.T) {
	tests := []struct{ in, pkg, short string }{
		{"github.com/yornik/go-practice/exercises/26-logfmt.Parse(...)", "github.com/yornik/go-practice/exercises/26-logfmt", "Parse"},
		{"example.com/x.(*SyntaxError).Error(0xc000010000)", "example.com/x", "(*SyntaxError).Error"},
		{"example.com/y.Count.func1()", "example.com/y", "Count.func1"},
		{"testing.(*T).Run", "testing", "(*T).Run"},
		{"panic({0x800830?, 0x61ed30?})", "", "panic"},
		{"main.main()", "main", "main"},
	}
	for _, tc := range tests {
		if got := funcPackage(tc.in); got != tc.pkg {
			t.Errorf("funcPackage(%q) = %q, want %q", tc.in, got, tc.pkg)
		}
		if got := shortFunc(tc.in); got != tc.short {
			t.Errorf("shortFunc(%q) = %q, want %q", tc.in, got, tc.short)
		}
	}
}

func TestNoFailure(t *testing.T) {
	for _, out := range []string{"", "=== RUN   TestA\n--- PASS: TestA (0.00s)\n", "ok  \texample.com/x\t0.01s\n"} {
		if exps := ExplainTestOutput("TestA", out); exps != nil {
			t.Errorf("ExplainTestOutput(%q) = %v, want nil", out, ids(exps))
		}
	}
	exps := ExplainTestOutput("", "--- FAIL: TestQuiet (0.00s)\n")
	if len(exps) != 1 || exps[0].ID != GuideFailingTest || exps[0].Raw.Test != "TestQuiet" {
		t.Errorf("silent failure: got %v", exps)
	}
}

// TestExit checks how an exit without a panic (os.Exit, log.Fatal) is
// found. go test -json gives the lines after "=== RUN" to the test that
// was running; package output and plain output end with go test's FAIL
// line. The fixtures run the real cases; these cover the borders.
func TestExit(t *testing.T) {
	const (
		fatal  = "2026/10/17 11:28:32 missing address\n"
		goFail = "FAIL\texample.com/x\t0.004s\n"
		// A watchdog such as the guard of the course's 33-ratelimit tests
		// prints every goroutine before it exits.
		dump = "\nTestInc is still running after 10s: a call is blocked.\nStacks of all goroutines:\n\n" +
			"goroutine 7 [sync.Mutex.Lock]:\n" +
			"sync.(*Mutex).Lock(...)\n\t/usr/local/go/src/sync/mutex.go:46\n" +
			"example.com/x.(*Counter).Get(0xc000012345)\n\t/work/x.go:13 +0x5a\n" +
			"testing.tRunner(0xc0000a4000, 0x5f2e10)\n\t/usr/local/go/src/testing/testing.go:2195 +0xea\n" +
			"created by testing.(*T).Run in goroutine 1\n\t/usr/local/go/src/testing/testing.go:2260 +0x4d4\n\n" +
			"goroutine 9 [running]:\n" +
			"example.com/x.guard.func1()\n\t/work/x_test.go:20 +0x71\n" +
			"...additional frames elided...\n" +
			"created by time.goFunc\n\t/usr/local/go/src/time/sleep.go:182 +0x2d\n\n"
	)
	tests := []struct {
		name, test, out  string
		id, rawTest, msg string
	}{
		{"log.Fatal in a test", "TestPort", "=== RUN   TestPort\n" + fatal,
			"runtime-exit-log-fatal", "TestPort", strings.TrimSpace(fatal)},
		{"os.Exit in a subtest", "TestPort/empty", "=== RUN   TestPort/empty\nmissing address\n",
			"runtime-exit", "TestPort/empty", "missing address"},
		{"the last line counts, after an assertion", "TestPort", "=== RUN   TestPort\n    x_test.go:7: Port() = 1, want 2\n" + fatal,
			"runtime-exit-log-fatal", "TestPort", strings.TrimSpace(fatal)},
		{"log.Fatal in init", "", fatal + goFail,
			"runtime-exit-log-fatal", "", strings.TrimSpace(fatal)},
		{"a silent exit in init", "", "exit status 1\n" + goFail,
			"runtime-exit", "", "exit status 1"},
		{"plain go test -v names the running test", "", "=== RUN   TestOK\n--- PASS: TestOK (0.00s)\n=== RUN   TestPort\nmissing address\n" + goFail + "FAIL\n",
			"runtime-exit", "TestPort", "missing address"},
		{"plain go test -v after the last result", "", "=== RUN   TestOK\n--- PASS: TestOK (0.00s)\nbye\n" + goFail,
			"runtime-exit", "", "bye"},
		{"plain go test", "", fatal + goFail + "FAIL\n",
			"runtime-exit-log-fatal", "", strings.TrimSpace(fatal)},
		{"log.Fatal with a message of several lines", "TestPort",
			"=== RUN   TestPort\n    x_test.go:5: start\n2026/10/17 11:28:32 invalid configuration:\nPORT: bad\n\tDB_URL: required\n",
			"runtime-exit-log-fatal", "TestPort", "2026/10/17 11:28:32 invalid configuration:\nPORT: bad\n\tDB_URL: required"},
		{"a log line in an earlier paragraph", "TestPort", "=== RUN   TestPort\n" + fatal + "\nmissing port\n",
			"runtime-exit", "TestPort", "missing port"},
		{"a line after a goroutine dump", "TestInc", "=== RUN   TestInc\n" + dump + fatal,
			"runtime-exit-log-fatal", "TestInc", strings.TrimSpace(fatal)},
		{"a watchdog's goroutine dump", "TestInc/once", "=== RUN   TestInc/once\n" + dump,
			GuideStackTrace, "TestInc/once", "TestInc is still running after 10s: a call is blocked.\nStacks of all goroutines:"},
		{"a goroutine dump without a line before it", "TestInc", "=== RUN   TestInc\n" + dump[strings.Index(dump, "goroutine 7"):],
			GuideStackTrace, "TestInc", "goroutine 7 [sync.Mutex.Lock]:"},
		{"package output with a goroutine dump", "", dump + goFail,
			GuideStackTrace, "", "TestInc is still running after 10s: a call is blocked.\nStacks of all goroutines:"},
		{"Windows line ends", "TestPort", "=== RUN   TestPort\r\nmissing address\r\n",
			"runtime-exit", "TestPort", "missing address"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exps := ExplainTestOutput(tc.test, tc.out)
			if len(exps) == 0 {
				t.Fatal("no explanation")
			}
			e := exps[0]
			if e.ID != tc.id || e.Raw.Test != tc.rawTest || e.Raw.Message != tc.msg || e.Raw.Source != SourceRuntime {
				t.Errorf("got %s test %q message %q (source %s), want %s test %q message %q",
					e.ID, e.Raw.Test, e.Raw.Message, e.Raw.Source, tc.id, tc.rawTest, tc.msg)
			}
			if tc.id == GuideStackTrace && (e.Raw.File != "x.go" || e.Raw.Line != 13) {
				t.Errorf("got %s:%d, want x.go:13, where the test goroutine hangs", e.Raw.File, e.Raw.Line)
			}
			if strings.Contains(tc.out, "want 2") && len(exps) != 2 {
				t.Errorf("got %v, want the exit and then the assertion", ids(exps))
			}
		})
	}

	noExit := []struct{ name, test, out string }{
		{"a parent that waits in t.Run", "TestPort", "=== RUN   TestPort\n"},
		{"a test that reported", "TestPort", "=== RUN   TestPort\nmissing address\n--- FAIL: TestPort (0.00s)\n"},
		{"a test that passed", "TestPort", "=== RUN   TestPort\nhello\n--- PASS: TestPort (0.00s)\n"},
		{"a binary killed from outside", "TestPort", "=== RUN   TestPort\nsignal: killed\n"},
		{"output cut in a line", "TestPort", "=== RUN   TestPort\nmissing addr"},
		{"output of another test", "TestPort", "=== RUN   TestOther\nmissing address\n"},
		{"lines without framing", "TestPort", "missing address\n"},
		{"a panic", "TestPort", "=== RUN   TestPort\npanic: boom\n\ngoroutine 7 [running]:\n"},
		{"package output of a failed test", "", "FAIL\n" + goFail},
		{"package output of a passing run", "", "PASS\nok  \texample.com/x\t0.01s\n"},
		{"package output without lines: maybe a kill", "", goFail},
		{"package output of a failed build", "", "FAIL\texample.com/x [build failed]\n"},
		{"go test killed the binary", "", "*** Test killed: ran too long (11m0s).\n" + goFail},
		{"plain output of a binary killed from outside", "", "=== RUN   TestKill\nsignal: killed\n" + goFail},
	}
	for _, tc := range noExit {
		t.Run(tc.name, func(t *testing.T) {
			for _, f := range scanTestOutput(tc.test, tc.out) {
				if strings.HasPrefix(f.text, "exit: ") {
					t.Errorf("finding %+v, want no exit", f)
				}
			}
			for _, e := range ExplainTestOutput(tc.test, tc.out) {
				if strings.HasPrefix(e.ID, "runtime-exit") {
					t.Errorf("ExplainTestOutput gave %s", e.ID)
				}
			}
		})
	}
}

func TestGuide(t *testing.T) {
	for _, id := range []string{GuideErrorMessage, GuideStackTrace, GuideRaceReport, GuideFailingTest} {
		g, ok := Guide(id)
		if !ok || g.ID != id || !g.Generic || g.HTML == "" || strings.HasPrefix(g.Title, "Guide") {
			t.Errorf("Guide(%q) = %+v, %v", id, g, ok)
		}
	}
	if _, ok := Guide("compile-undefined"); ok {
		t.Error("Guide returned an entry that is not a guide")
	}
}

func TestBrokenCatalogStillExplains(t *testing.T) {
	var c *catalogData
	e := c.explain(finding{source: SourceCompile, guide: GuideErrorMessage, raw: Raw{Message: "undefined: x"}})
	if e.Raw.Message != "undefined: x" || e.HTML == "" {
		t.Errorf("got %+v", e)
	}
}

func ids(exps []Explanation) []string {
	var s []string
	for _, e := range exps {
		s = append(s, e.ID)
	}
	return s
}

// TestFallback checks that an entry marked fallback loses to any other
// pattern, whatever the order of the files.
func TestFallback(t *testing.T) {
	c := testCatalog(t, map[string]string{
		"test/test-a-got-want": entryFile("test-a-got-want", "test",
			"match: ^(?P<call>.+) = (?P<got>.+), want (?P<want>.+)$\nfallback: true\ntitle: got and want\n", ""),
		"test/test-b-stub": entryFile("test-b-stub", "test", "match: TODO: implement\ntitle: stub\n", ""),
	})
	tests := []struct{ msg, id string }{
		{"Do = Do panicked: TODO: implement, want an error", "test-b-stub"},
		{"Do = 3, want 4", "test-a-got-want"},
		{"something else", GuideFailingTest},
	}
	for _, tc := range tests {
		f := finding{source: SourceTest, guide: GuideFailingTest, raw: Raw{Message: tc.msg}}
		if e := c.explain(f); e.ID != tc.id {
			t.Errorf("%q: got %s, want %s", tc.msg, e.ID, tc.id)
		}
	}
}

// TestHelperPanicAssertion checks the line that a helper with t.Helper()
// prints when it recovers a panic: it names the stub file, not a test file.
// The testing package's own lines are not assertions.
func TestHelperPanicAssertion(t *testing.T) {
	out := "=== RUN   TestBurn\n" +
		"    slo.go:76: panicked: TODO: implement\n" +
		"        goroutine 7 [running]:\n" +
		"        \t/work/slo.go:76 +0x88\n" +
		"    testing.go:1712: race detected during execution of test\n" +
		"--- FAIL: TestBurn (0.00s)\n"
	fs := assertFindings("", strings.Split(out, "\n"))
	if len(fs) != 1 {
		t.Fatalf("%d findings, want 1: %+v", len(fs), fs)
	}
	r := fs[0].raw
	if r.File != "slo.go" || r.Line != 76 || r.Test != "TestBurn" || !strings.HasPrefix(r.Message, "panicked: TODO: implement\ngoroutine 7") {
		t.Errorf("raw = %+v", r)
	}
	if exps := ExplainTestOutput("TestBurn", out); len(exps) == 0 || exps[0].ID != "test-panicked-stub" {
		t.Errorf("ExplainTestOutput = %v, want test-panicked-stub first", ids(exps))
	}
}

// TestCatalogBoundaries runs real messages of Go 1.27.2 and of the course
// tests through the real catalog. Each one sits on the border between 2
// entries, or must fall back to a guide, so a pattern that grows too broad
// fails here. The fixtures cover the main message of each entry.
func TestCatalogBoundaries(t *testing.T) {
	tests := []struct {
		source, code, msg, id string
	}{
		// "else" after a statement on the same line is not an else on a new line.
		{SourceCompile, "", "syntax error: unexpected keyword else at end of statement", "compile-syntax-unexpected-token"},
		{SourceCompile, "", "syntax error: unexpected keyword else, expected }", "compile-syntax-else-newline"},
		{SourceCompile, "", "expected ';', found 'else'", "compile-syntax-unexpected-token"},
		// switch x with { on the next line: 2 errors, each with its own entry.
		{SourceCompile, "", "x (variable of type int) is not used", "compile-value-not-used"},
		{SourceCompile, "", "cannot convert 1 (untyped int constant) to type bool", "compile-cannot-convert"},
		// The editor phrases an assignment as a condition in 3 ways.
		{SourceCompile, "", "expected switch expression, found assignment (missing parentheses around composite literal?)", "compile-assignment-as-condition"},
		{SourceCompile, "", "expected boolean or range expression, found simple statement (missing parentheses around composite literal?)", "compile-assignment-as-condition"},
		{SourceCompile, "", "syntax error: cannot use x++ as value", "compile-assignment-as-condition"},
		// A raw string that runs to the end of the file.
		{SourceCompile, "", "string not terminated", "compile-syntax-unterminated-string"},
		// declared and not used: err has its own entry, and only err.
		{SourceCompile, "", "declared and not used: err", "compile-shadowed-err"},
		{SourceCompile, "", "declared and not used: errs", "compile-declared-and-not-used"},
		{SourceCompile, "", "declared and not used: e", "compile-declared-and-not-used"},
		// An untyped constant on either side, and none.
		{SourceCompile, "", "invalid operation: s += 1 (mismatched types string and untyped int)", "compile-mismatched-untyped"},
		{SourceCompile, "", "invalid operation: sum / n (mismatched types float64 and int)", "compile-mismatched-types"},
		{SourceCompile, "", "invalid operation: x < y (mismatched types uint8 and int)", "compile-mismatched-types"},
		// "cannot use" with a reason after a colon names the interface problem.
		{SourceCompile, "", "cannot use n (variable of type int) as string value in argument to strings.ToUpper", "compile-cannot-use"},
		{SourceCompile, "", "cannot use 256 (untyped int constant) as byte value in variable declaration (overflows)", "compile-constant-overflow"},
		{SourceCompile, "", "cannot use 1.5 (untyped float constant) as int value in variable declaration (truncated)", "compile-constant-truncated"},
		{SourceCompile, "", "cannot use nil as Host value in return statement", "compile-nil-value"},
		{SourceCompile, "", "cannot use T{} (value of struct type T) as fmt.Stringer value in variable declaration: T does not implement fmt.Stringer (missing method String)", "compile-missing-method"},
		{SourceCompile, "", "cannot use NotFoundError{…} (value of struct type NotFoundError) as error value in return statement: NotFoundError does not implement error (method Error has pointer receiver)", "compile-pointer-receiver"},
		{SourceCompile, "", "cannot use &Tally{} (value of type *Tally) as io.Writer value in variable declaration: *Tally does not implement io.Writer (wrong type for method Write)\n\t\thave Write([]byte) int\n\t\twant Write([]byte) (int, error)", "compile-wrong-method-signature"},
		{SourceCompile, "", "cannot range over ch (variable of type chan<- int): receive from send-only channel chan<- int", "compile-channel-direction"},
		{SourceCompile, "", "cannot range over s (variable of struct type Set)", "compile-cannot-range"},
		// Messages without an entry fall back to the guide.
		{SourceCompile, "", "missing key in map literal", GuideErrorMessage},
		{SourceCompile, "", "cannot use _ as value or type", GuideErrorMessage},
		{SourceCompile, "", "invalid break label L", GuideErrorMessage},
		{SourceCompile, "", "initialization cycle for a", GuideErrorMessage},
		{SourceCompile, "", "invalid operation: cannot send to non-channel int x (variable of type int)", GuideErrorMessage},
		// go test prints vet findings without the analyzer.
		{SourceVet, "", "fmt.Printf format %d has arg name of wrong type string", "vet-printf-wrong-type"},
		{SourceVet, "", "fmt.Sprintf format %i has unknown verb i", GuideErrorMessage},
		// Course test messages: a stub panic wins over got/want, a real
		// panic too, and a message about a panic that the test caused itself
		// stays a got/want.
		{SourceTest, "", "goroutine 0: Do = Do panicked: TODO: implement, want a *ExhaustedError", "test-panicked-stub"},
		{SourceTest, "", "1st Do(\"user:42\") = \"\", <nil> (panic: TODO: implement), want the error connection refused", "test-panicked-stub"},
		{SourceTest, "", "Do panicked before fn was called: TODO: implement", "test-panicked-stub"},
		{SourceTest, "", "New(0, 1, clock) panicked with \"TODO: implement\", want a Limiter", "test-panicked-stub"},
		{SourceTest, "", "panicked: TODO: implement\ngoroutine 7 [running]:", "test-panicked-stub"},
		{SourceTest, "", "goroutine 0: Do = Do panicked: runtime error: index out of range [1] with length 0, want a *ExhaustedError", "test-panicked"},
		{SourceTest, "", "panicked: assignment to entry in nil map", "test-panicked"},
		{SourceTest, "", "3 of 8 goroutines panicked, for example with: send on closed channel", "test-panicked"},
		{SourceTest, "", "handler panicked with \"boom\"; what came out of AccessLog = <nil>, want the same panic", "test-got-want"},
		{SourceTest, "", "PanicError.Stack does not mention crashingTask, the function that panicked. Stack:\ngoroutine 1", GuideFailingTest},
		{SourceTest, "", "fn was called 5 times, want 4: the 4th call panicked, so Map must not start a 5th", GuideFailingTest},
		// An exit: the default log prefix means log.Fatal, anything else
		// is any exit. An assertion that reads the same stays a test text.
		{SourceRuntime, "", "exit: 2026/10/17 11:28:32 missing address", "runtime-exit-log-fatal"},
		{SourceRuntime, "", "exit: 2026/10/17 11:28:32.123456 x.go:8: missing address", "runtime-exit-log-fatal"},
		{SourceRuntime, "", "exit: 11:28:32 missing address", "runtime-exit"},
		{SourceRuntime, "", "exit: exit status 1", "runtime-exit"},
		{SourceRuntime, "", "exit: 2026/10/17 11:28:32 invalid configuration:\nPORT: bad\nDB_URL: required", "runtime-exit-log-fatal"},
		{SourceRuntime, "", "exit: bad port\nusage: x", "runtime-exit"},
		{SourceRuntime, "", "panic: unexpected call to os.Exit(0) during test", "runtime-exit-zero"},
		{SourceRuntime, "", "panic: unexpected call to os.Exit(1) during test", GuideErrorMessage},
		{SourceTest, "", "exit: 2026/10/17 11:28:32 missing address", GuideFailingTest},
	}
	c := catalog()
	for _, tc := range tests {
		guide := GuideErrorMessage
		if tc.source == SourceTest {
			guide = GuideFailingTest
		}
		f := finding{source: tc.source, code: tc.code, guide: guide, raw: Raw{Message: tc.msg}}
		if e := c.explain(f); e.ID != tc.id {
			t.Errorf("%s %q: got %s, want %s", tc.source, tc.msg, e.ID, tc.id)
		}
	}
}

// TestFrameWithRegisters checks the frame lines of a fatal error such as a
// stack overflow, which end with the frame, stack and program counters.
func TestFrameWithRegisters(t *testing.T) {
	lines := []string{
		"runtime: goroutine stack exceeds 1000000-byte limit",
		"fatal error: stack overflow",
		"",
		"runtime stack:",
		"runtime.throw({0x5a3b1e?, 0x0?})",
		"\t/usr/local/go/src/runtime/panic.go:1243 +0x48 fp=0x7ffdedcdff68 sp=0x7ffdedcdff38 pc=0x486388",
		"example.com/fixture.Reach(0x3f7a579e0560, {0x5a0f0b, 0x1})",
		"\t/work/x.go:7 +0xb1 fp=0x3f7a579e04e8 sp=0x3f7a579e0488 pc=0x543911",
	}
	fs := crashFindings("TestReach", lines)
	if len(fs) != 1 || fs[0].raw.File != "x.go" || fs[0].raw.Line != 7 || fs[0].builtins["func"] != "Reach" {
		t.Fatalf("findings = %+v", fs)
	}
}
