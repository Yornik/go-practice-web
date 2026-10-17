// Package explain turns what the Go tools print (compiler errors, go vet and
// staticcheck findings, panics, race reports, failing tests) into
// explanations that teach.
//
// The explanations come from a reviewed catalog of Markdown files embedded
// in the binary (catalog/<source>/<id>.md, see catalog/README.md). The
// course forbids LLM help, so nothing here is generated: every entry matches
// one kind of message, and a message without an entry gets one of the
// generic guides ("How to read a Go error message", ...).
//
// The catalog is loaded and validated when the package initialises. Invalid
// entries are left out and reported by [Check], so a broken entry cannot
// stop the web server; the package tests fail on it instead.
package explain

import (
	"strings"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// Sources of catalog entries. An entry matches only messages of its own
// source; see catalog/README.md for what each one receives.
const (
	SourceCompile     = "compile"     // compiler and type errors, gofmt syntax errors
	SourceVet         = "vet"         // go vet findings, matched by analyzer name
	SourceStaticcheck = "staticcheck" // staticcheck findings, matched by check code
	SourceRuntime     = "runtime"     // panics and fatal errors in test output
	SourceRace        = "race"        // race detector reports
	SourceTest        = "test"        // failing assertions, Example mismatches, timeouts
	SourceBuild       = "build"       // go command errors without a position (go.mod, imports)
	SourceGuide       = "guide"       // the generic guides; they never match by themselves
)

// IDs of the generic guides, the fallbacks for messages without an entry.
const (
	GuideErrorMessage = "guide-error-message"
	GuideStackTrace   = "guide-stack-trace"
	GuideRaceReport   = "guide-race-report"
	GuideFailingTest  = "guide-failing-test"
)

// CourseURLPrefix is where the site serves an exercise (docs/DESIGN.md,
// "HTTP routes"). An entry's course exercise links there.
const CourseURLPrefix = "/learn/"

// Explanation teaches what one message means. It is the "explain" object of
// docs/DESIGN.md, "Learning-first feedback".
type Explanation struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	// Title is plain text with the captured values filled in. Escape it like
	// any other text (textContent, html/template); it is not HTML.
	Title string `json:"title"`
	// HTML is the rendered body, safe to insert as HTML: the Markdown is
	// rendered without raw HTML and every captured value is escaped. It ends
	// with the entry's links, so it is complete on its own.
	HTML  string `json:"html"`
	Links []Link `json:"links"`
	// Course is the slug of the exercise that teaches the concept, for
	// example "12-workerpool", or empty.
	Course string `json:"course,omitempty"`
	// Generic is true when no entry matched and this is a generic guide.
	Generic bool `json:"generic,omitempty"`
	// Raw is the original message split into its parts, so the UI can show
	// it with labels next to the explanation.
	Raw Raw `json:"raw"`
}

// Link is one "Learn more" link of an entry.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Raw holds the parts of the message an explanation is about.
type Raw struct {
	// Source is the kind of message: one of the Source constants except
	// SourceGuide.
	Source string `json:"source"`
	// File is a base name such as "retry.go" or "retry_test.go"; empty when
	// the message has no position. It can name a file of the Go runtime or
	// the standard library, such as "panic.go": a test helper that recovers
	// a runtime error reports the runtime's line.
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	// Openable is true when File is a file of the exercise, which the
	// editor can open at Line. The engine does not know the exercise and
	// leaves it false; the web server sets it. A UI shows a File that is
	// not openable as plain text.
	Openable bool `json:"openable,omitempty"`
	// Code is the vet analyzer or the staticcheck check code, if known.
	Code string `json:"code,omitempty"`
	// Test is the test or example that failed, for test output.
	Test string `json:"test,omitempty"`
	// Message is the message without its position, for example
	// "undefined: foo" or "panic: TODO: implement".
	Message string `json:"message"`
	// Captures holds the named values taken from the message (the entry's
	// regexp groups and the built-in values such as "func"). The UI can use
	// them to label the parts of Message.
	Captures map[string]string `json:"captures,omitempty"`
}

// Explain explains one diagnostic of a check or format response. It matches
// the catalog by source, then by code (vet analyzer or staticcheck code) and
// message; without a match it returns the guide "How to read a Go error
// message".
func Explain(d runnerapi.Diagnostic) Explanation {
	source, code, msg := normalizeDiagnostic(d)
	f := finding{
		source: source,
		code:   code,
		guide:  GuideErrorMessage,
		raw: Raw{
			Source:  source,
			File:    d.File,
			Line:    d.Line,
			Column:  d.Column,
			Code:    code,
			Message: msg,
		},
	}
	f.builtins = map[string]string{
		"file":   d.File,
		"line":   itoa(d.Line),
		"column": itoa(d.Column),
		"code":   code,
	}
	return catalog().explain(f)
}

// ExplainTestOutput explains the output of one test (test may be empty for
// package-level output). It finds, in this order: a build failure, a test
// timeout, a fatal runtime error such as a deadlock, race reports, a panic
// (including the course stub's "TODO: implement"), an Example whose output
// differs, and failing assertions (file_test.go:NN: lines, or file.go:NN:
// when a test helper reports a panic at its line in a stub). The result holds
// at most 1 explanation per catalog entry, the first occurrence, because
// the first failure matters most. A failed test without any of these gets
// the guide "How to read a failing test". Output without a failure gives
// nil.
func ExplainTestOutput(test, output string) []Explanation {
	findings := scanTestOutput(test, normalizeNewlines(output))
	c := catalog()
	var out []Explanation
	seen := make(map[string]bool)
	for _, f := range findings {
		e := c.explain(f)
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	return out
}

// ExplainBuildOutput parses the compiler output of a failed build (the
// "build" event of a test run, or go build/go vet text output) into
// diagnostics and explains each one. The 2 slices are parallel:
// explanations[i] explains diagnostics[i]. File names are base names, as in
// check responses. Findings of go vet that go test reports before it runs
// the tests have source "vet" and no code, because the text output does not
// name the analyzer.
func ExplainBuildOutput(output string) ([]runnerapi.Diagnostic, []Explanation) {
	diags := parseBuildOutput(normalizeNewlines(output))
	exps := make([]Explanation, len(diags))
	for i, d := range diags {
		exps[i] = Explain(d)
	}
	return diags, exps
}

// Ref is a file:line reference in tool output that points into the
// exercise: a stub file or a test file. References into the standard
// library, the module cache and generated files are left out.
type Ref struct {
	// File is the base name, for example "retry_test.go".
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
	// Test is true for a _test.go file.
	Test bool `json:"test"`
	// Start and End are the byte offsets of the reference text (path, line
	// and column) in the output, so the UI can turn it into a link.
	Start int `json:"start"`
	End   int `json:"end"`
}

// Refs returns every file:line reference in output that points into the
// exercise, in order of appearance.
func Refs(output string) []Ref {
	var refs []Ref
	for _, m := range refRE.FindAllStringSubmatchIndex(output, -1) {
		path := output[m[2]:m[3]]
		base := output[m[4]:m[5]]
		if isStdPath(path) || base == "_testmain.go" {
			continue
		}
		// The testing package prints its own file bare, for example
		// "testing.go:1867: race detected during execution of test".
		if path == "testing.go" {
			continue
		}
		r := Ref{
			File:  base,
			Line:  atoi(output[m[6]:m[7]]),
			Test:  strings.HasSuffix(base, "_test.go"),
			Start: m[2],
			End:   m[7],
		}
		if m[8] >= 0 {
			r.Column = atoi(output[m[8]:m[9]])
			r.End = m[9]
		}
		refs = append(refs, r)
	}
	return refs
}

// Guide returns one of the generic guides by ID (GuideErrorMessage, ...),
// for pages that link to a guide without a message.
func Guide(id string) (Explanation, bool) {
	c := catalog()
	e, ok := c.byID[id]
	if !ok || e.source != SourceGuide {
		return Explanation{}, false
	}
	return e.explanation(nil, Raw{}, true), true
}

// Check reports the catalog entries that failed to load. The package tests
// fail on any of them; the web server can log them at start.
func Check() error {
	_, err := loadDefault()
	return err
}

// normalizeDiagnostic decides which catalog source a diagnostic is matched
// against and separates a check code that a runner left in the message.
func normalizeDiagnostic(d runnerapi.Diagnostic) (source, code, msg string) {
	source, code = d.Source, d.Code
	msg = strings.TrimRight(normalizeNewlines(d.Message), " \t\n")
	if code == "" && (source == runnerapi.SourceStaticcheck || source == "") {
		// staticcheck's text format ends the message with the code, for
		// example "this value of x is never used (SA4006)".
		if m := trailingCodeRE.FindStringSubmatch(msg); m != nil {
			code = m[2]
			msg = m[1]
		}
	}
	switch {
	case source == runnerapi.SourceGofmt:
		// gofmt only reports syntax errors, which are compile errors.
		source = SourceCompile
	case code == "compile":
		// staticcheck reports type errors with the code "compile".
		source, code = SourceCompile, ""
	case source == "":
		source = SourceCompile
	}
	if source == SourceCompile || source == SourceBuild {
		code = ""
	}
	return source, code, msg
}
