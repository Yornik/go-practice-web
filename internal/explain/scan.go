package explain

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

var (
	// posRE is a compiler or vet line: "./x.go:12:5: message". go vet
	// prefixes type errors with "vet: ".
	posRE = regexp.MustCompile(`^(vet: )?((?:[A-Za-z]:)?[^\s:]*\.go):(\d+)(?::(\d+))?: (.*)$`)
	// locRE is the location line of a stack frame: "\t/path/x.go:12 +0x1d".
	// A fatal error such as a stack overflow adds " fp=0x.. sp=0x.. pc=0x..".
	locRE = regexp.MustCompile(`^\s+(\S+\.go):(\d+)(?:\s+\+0x[0-9a-f]+)?(?:\s+fp=\S+ sp=\S+ pc=\S+)?\s*$`)
	// refRE finds file:line references anywhere in output. Group 1 is the
	// path, 2 the base name, 3 the line, 4 the column.
	refRE = regexp.MustCompile(`(?m)(?:^|[\s(])((?:[A-Za-z]:)?(?:[^\s:()]*/)?([A-Za-z0-9_.+-]+\.go)):(\d+)(?::(\d+))?\b`)

	// assertRE is a line that a test reported with t.Errorf and friends:
	// "    retry_test.go:42: message". A helper that calls t.Helper() and
	// recovers a panic reports the line of the panic itself, in a stub
	// file: "    slo.go:76: panicked: TODO: implement".
	assertRE      = regexp.MustCompile(`^(\s+)([A-Za-z0-9_.+-]+\.go):(\d+): ?(.*)$`)
	testLineRE    = regexp.MustCompile(`^\s*(?:=== (?:RUN|CONT|PAUSE|NAME)\s+|--- (?:FAIL|PASS|SKIP): )(\S+)`)
	failLineRE    = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)
	runLineRE     = regexp.MustCompile(`^=== (?:RUN|CONT|NAME)\s+(\S+)`)
	buildFailRE   = regexp.MustCompile(`(?m)^FAIL\s+\S+ \[(?:build|setup) failed\]$`)
	buildHeadRE   = regexp.MustCompile(`(?m)^# \S.*\n(?:vet: )?\S+\.go:\d+`)
	recoveredRE   = regexp.MustCompile(`\s*\[recovered[^\]]*\]\s*$`)
	raceAccessRE  = regexp.MustCompile(`^(Previous )?([A-Za-z ]+?) at 0x[0-9a-f]+ by (?:main goroutine|goroutine \d+):$`)
	timeoutRE     = regexp.MustCompile(`^panic: test timed out after \S+`)
	runningTestRE = regexp.MustCompile(`^\t\t(\S+) \(`)
	goroutineRE   = regexp.MustCompile(`^goroutine \d+ .*:$`)
	framingRE     = regexp.MustCompile(`^=== (?:RUN|CONT|PAUSE|NAME)\s+(\S+)`)
	resultLineRE  = regexp.MustCompile(`^\s*--- (?:FAIL|PASS|SKIP): (\S+)`)
	// goFailRE is the line go test adds to the package output when a test
	// binary failed: "FAIL\texample.com/x\t0.005s". A build failure has
	// " [build failed]" instead of the time.
	goFailRE = regexp.MustCompile(`^FAIL\t\S+\t\S+$`)
	// logPrefixRE is the date and time that the standard logger writes
	// before a message: "2026/10/17 11:28:32 ", with microseconds when the
	// flags ask for them.
	logPrefixRE   = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? `)
	raceSeparator = "=================="
)

// parseBuildOutput turns compiler and go command output into diagnostics.
// A "# [pkg]" header starts the go vet findings that go test reports;
// any other "# pkg" header starts compiler output. Lines that start with a
// tab or "note: " continue the previous message.
func parseBuildOutput(out string) []runnerapi.Diagnostic {
	var diags []runnerapi.Diagnostic
	source := SourceCompile
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(line, "# "):
			source = SourceCompile
			if strings.HasPrefix(line, "# [") {
				source = SourceVet
			}
			continue
		case isSummaryLine(line):
			continue
		case (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "note: ")) && len(diags) > 0:
			diags[len(diags)-1].Message += "\n" + line
			continue
		}
		d := runnerapi.Diagnostic{Severity: runnerapi.SeverityError, Source: source, Message: trimmed}
		if m := posRE.FindStringSubmatch(line); m != nil {
			if m[5] == "too many errors" {
				// The compiler stopped after 10 errors; not a problem of its own.
				continue
			}
			d.File = path.Base(m[2])
			d.Line = atoi(m[3])
			d.Column = atoi(m[4])
			d.Message = m[5]
			if m[1] != "" {
				// go vet could not type-check the package: a compile error.
				d.Source = SourceCompile
			}
		} else {
			d.Source = SourceCompile
		}
		diags = append(diags, d)
	}
	return diags
}

func isSummaryLine(line string) bool {
	return strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "ok ") ||
		strings.HasPrefix(line, "ok\t") || line == "too many errors" ||
		strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "=== ")
}

// frame is one function call in a stack trace or race report.
type frame struct {
	fn   string // as printed, for example "example.com/x.Parse(...)"
	path string
	line int
}

// parseFrames reads the frames of a goroutine dump or a race report: a
// function line followed by an indented "path.go:NN +0x1f" line.
func parseFrames(lines []string) []frame {
	var frames []frame
	for i, l := range lines {
		m := locRE.FindStringSubmatch(l)
		if m == nil || i == 0 {
			continue
		}
		fn := strings.TrimSpace(lines[i-1])
		fn = strings.TrimPrefix(fn, "created by ")
		if before, _, ok := strings.Cut(fn, " in goroutine "); ok {
			fn = before
		}
		frames = append(frames, frame{fn: fn, path: m[1], line: atoi(m[2])})
	}
	return frames
}

// user reports whether the frame is in the learner's or the exercise's
// code, not in the standard library or the generated test main.
func (f frame) user() bool {
	if isStdPath(f.path) || path.Base(f.path) == "_testmain.go" {
		return false
	}
	pkg := funcPackage(f.fn)
	if pkg == "main" {
		return true
	}
	first, _, _ := strings.Cut(pkg, "/")
	// Standard library import paths have no dot in their first element.
	return strings.Contains(first, ".")
}

func firstUserFrame(frames []frame) (frame, bool) {
	for _, f := range frames {
		if f.user() {
			return f, true
		}
	}
	return frame{}, false
}

// funcPackage returns the import path of a frame's function:
// "example.com/x" for "example.com/x.(*T).M(0x1)".
func funcPackage(fn string) string {
	slash := strings.LastIndex(fn, "/")
	dot := strings.Index(fn[slash+1:], ".")
	if dot < 0 {
		return ""
	}
	return fn[:slash+1+dot]
}

// shortFunc returns the function name without the import path and the
// arguments: "(*SyntaxError).Error" for
// "example.com/x.(*SyntaxError).Error(0xc000010000)".
func shortFunc(fn string) string {
	pkg := funcPackage(fn)
	name := strings.TrimPrefix(fn[len(pkg):], ".")
	if strings.HasSuffix(name, ")") {
		depth := 0
		for i := len(name) - 1; i >= 0; i-- {
			switch name[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					return name[:i]
				}
			}
		}
	}
	return name
}

// isStdPath reports paths in GOROOT or the module cache: the toolchain
// lives in /usr/local/go/src or in the module cache as golang.org/toolchain.
func isStdPath(p string) bool {
	return strings.Contains(p, "/pkg/mod/") || strings.Contains(p, "/go/src/") ||
		strings.Contains(p, "/toolchain@") || strings.HasPrefix(p, "$GOROOT")
}

// scanTestOutput finds everything worth explaining in the output of 1 test,
// most important first.
func scanTestOutput(test, out string) []finding {
	if strings.TrimSpace(out) == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	var fs []finding
	fs = append(fs, buildFindings(out)...)
	fs = append(fs, crashFindings(test, lines)...)
	if len(fs) == 0 {
		// A panic or a fatal error explains why a test has no result.
		fs = append(fs, exitFindings(test, lines)...)
	}
	fs = append(fs, raceFindings(test, lines)...)
	fs = append(fs, exampleFindings(test, lines)...)
	fs = append(fs, assertFindings(test, lines)...)
	if len(fs) == 0 {
		name := test
		for _, l := range lines {
			if m := failLineRE.FindStringSubmatch(l); m != nil {
				if name == "" {
					name = m[1]
				}
				fs = append(fs, finding{
					source: SourceTest,
					guide:  GuideFailingTest,
					raw:    Raw{Source: SourceTest, Test: name, Message: strings.TrimSpace(l)},
				})
				break
			}
		}
	}
	return fs
}

// buildFindings explains a build failure printed inside test output.
func buildFindings(out string) []finding {
	if !buildFailRE.MatchString(out) && !buildHeadRE.MatchString(out) {
		return nil
	}
	// Keep only the build part: headers, positioned lines, go command
	// errors and their continuation lines.
	var keep []string
	inBuild := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "# "), strings.HasPrefix(l, "go: "):
			inBuild = true
		case strings.TrimSpace(l) == "", isSummaryLine(l):
			inBuild = false
			continue
		}
		if inBuild {
			keep = append(keep, l)
		}
	}
	var fs []finding
	for _, d := range parseBuildOutput(strings.Join(keep, "\n")) {
		source, code, msg := normalizeDiagnostic(d)
		fs = append(fs, finding{
			source: source,
			code:   code,
			guide:  GuideErrorMessage,
			raw: Raw{Source: source, File: d.File, Line: d.Line, Column: d.Column,
				Code: code, Message: msg},
			builtins: map[string]string{"file": d.File, "line": itoa(d.Line), "column": itoa(d.Column), "code": code},
		})
	}
	return fs
}

// crashFindings finds the 1 event that stops a test binary: a test timeout,
// a fatal runtime error (such as a deadlock) or a panic.
func crashFindings(test string, lines []string) []finding {
	for i, l := range lines {
		switch {
		case timeoutRE.MatchString(l):
			name := test
			for _, r := range lines[i+1:] {
				if m := runningTestRE.FindStringSubmatch(r); m != nil {
					if name == "" {
						name = m[1]
					}
					break
				}
			}
			f := stackFinding(SourceTest, GuideStackTrace, name, strings.TrimSpace(l), hangingGoroutines(lines[i+1:]))
			delete(f.builtins, "func")
			return []finding{f}
		case strings.HasPrefix(l, "fatal error: "):
			return []finding{stackFinding(SourceRuntime, GuideStackTrace, contextTest(test, lines, i, -1), strings.TrimSpace(l), lines[i+1:])}
		case strings.HasPrefix(l, "panic: "):
			msg := panicMessage(lines[i:])
			return []finding{stackFinding(SourceRuntime, GuideStackTrace, contextTest(test, lines, i, -1), msg, lines[i+1:])}
		}
	}
	return nil
}

// exitFindings finds a test binary that stopped without a panic before the
// test reported a result, as os.Exit and log.Fatal make it do. No line says
// so: the output stops after the last line the code printed, and go test
// adds its FAIL line to the package output, with "exit status 1" before it
// only when the binary printed nothing at all.
//
// With a test name, lines are that test's output (go test -json gives each
// test the lines after its "=== RUN"). The finding goes to the test when it
// started, never reported a result and printed a line. So a parent that
// waits in t.Run gets nothing and the subtest that ran gets the finding. A
// test that exits without printing leaves the output of such a parent, and
// package output like that of a test killed from outside ("signal: killed"
// goes to the test's output), so that exit gets no finding at all.
//
// Without a test name, lines are package output or plain go test output,
// which must end with go test's FAIL line; the finding goes to the test
// that its framing lines show running, if any.
//
// The finding's text for the catalog is "exit: " and the last line printed,
// or all lines of a log message of several lines. When that line ends a
// goroutine dump, the finding is the guide to stack traces instead (see
// dumpFinding).
func exitFindings(test string, lines []string) []finding {
	// Output cut in the middle of a line, such as by a cap on the output
	// kept per test, does not show where the binary stopped.
	if len(lines) < 2 || lines[len(lines)-1] != "" {
		return nil
	}
	end := len(lines) - 1
	if test == "" {
		if end = slices.IndexFunc(lines, goFailRE.MatchString); end < 0 {
			return nil
		}
	}
	started := test == ""
	running, lastAt := test, -1
	for i, l := range lines[:end] {
		trimmed := strings.TrimSpace(l)
		if m := framingRE.FindStringSubmatch(l); m != nil {
			started = started || m[1] == test
			// A parallel test that paused does not run.
			if test == "" && runLineRE.MatchString(l) {
				running = m[1]
			}
			continue
		}
		if m := resultLineRE.FindStringSubmatch(l); m != nil {
			if test != "" {
				return nil
			}
			if m[1] == running {
				running = ""
			}
			continue
		}
		switch {
		case trimmed == "":
		case trimmed == "PASS" || trimmed == "FAIL" || strings.HasPrefix(l, "ok "):
			// The testing package ran to its end.
			return nil
		default:
			lastAt = i
		}
	}
	if !started || lastAt < 0 {
		return nil
	}
	last := strings.TrimSpace(lines[lastAt])
	// go test prints "signal: killed" for a binary killed from outside, and
	// "*** Test killed" when it stopped the binary itself: no exit of the
	// code.
	if strings.HasPrefix(last, "signal: ") || strings.HasPrefix(last, "*** ") {
		return nil
	}
	if f, ok := dumpFinding(running, lines[:lastAt+1]); ok {
		return []finding{f}
	}
	// log.Fatal writes its message at once, and the log prefix goes on the
	// first line only: a message of several lines, such as an error from
	// errors.Join, is the last printed line and the lines above it up to
	// the one with the prefix.
	for i := lastAt; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" || testLineRE.MatchString(lines[i]) {
			break
		}
		if logPrefixRE.MatchString(lines[i]) {
			last = strings.TrimSpace(strings.Join(lines[i:lastAt+1], "\n"))
			break
		}
	}
	return []finding{{
		source:   SourceRuntime,
		guide:    GuideFailingTest,
		raw:      Raw{Source: SourceRuntime, Test: running, Message: last},
		text:     "exit: " + last,
		builtins: map[string]string{"test": running},
	}}
}

// dumpFinding handles an exit right after a goroutine dump, the last of
// lines. A watchdog prints one when a call never returns and then exits,
// as the guard of the course's 33-ratelimit tests does after 10 s: the
// code under test called no os.Exit, it hangs. So the finding is the guide
// to stack traces, at the first exercise frame of the goroutine that hangs,
// and its message is the paragraph printed just before the dump, which
// says why the watchdog fired.
func dumpFinding(test string, lines []string) (finding, bool) {
	last := len(lines) - 1
	if last < 1 || !locRE.MatchString(lines[last]) {
		return finding{}, false
	}
	header := -1
	for i := last; i >= 0 && inDump(lines, i); i-- {
		if goroutineRE.MatchString(lines[i]) {
			header = i
		}
	}
	if header < 0 {
		return finding{}, false
	}
	var para []string
	for i := header - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" && len(para) == 0 {
			continue
		}
		if l == "" || testLineRE.MatchString(lines[i]) {
			break
		}
		para = append(para, l)
	}
	slices.Reverse(para)
	msg := strings.Join(para, "\n")
	if msg == "" {
		msg = strings.TrimSpace(lines[header])
	}
	f := stackFinding(SourceRuntime, GuideStackTrace, test, msg, hangingGoroutines(lines[header:]))
	// The text is whatever the watchdog printed, so no entry may claim it:
	// a finding with the guide source searches no entries.
	f.source = SourceGuide
	return f, true
}

// inDump reports whether lines[i] can be part of a goroutine dump: a
// header, a frame (a function or "created by" line above its location
// line), elided frames, or a blank line between goroutines.
func inDump(lines []string, i int) bool {
	l := strings.TrimSpace(lines[i])
	return l == "" || l == "...additional frames elided..." || goroutineRE.MatchString(lines[i]) ||
		locRE.MatchString(lines[i]) || i+1 < len(lines) && locRE.MatchString(lines[i+1])
}

// hangingGoroutines returns the part of a test timeout's goroutine dump
// that shows where the test hangs. The testing package prints every
// goroutine, and the ones parked in the testing package itself only wait
// for the one that hangs: the alarm, the test runner in goroutine 1, a
// parent test in t.Run that waits for its subtest (its first exercise
// frame is the t.Run line of a table test), a parallel test that waits
// for its turn. Of the rest, the goroutines that run a test come first,
// then those the code started: an earlier test may have leaked one. When
// none of them has a frame in the exercise, it returns lines unchanged.
func hangingGoroutines(lines []string) []string {
	var tests, others []string
	add := func(block []string) {
		frames := parseFrames(block)
		switch {
		case len(frames) == 0 || funcPackage(frames[0].fn) == "testing":
		case slices.ContainsFunc(frames, func(f frame) bool { return strings.HasPrefix(f.fn, "testing.tRunner(") }):
			tests = append(tests, block...)
		default:
			others = append(others, block...)
		}
	}
	start := -1
	for i, l := range lines {
		if goroutineRE.MatchString(l) {
			if start >= 0 {
				add(lines[start:i])
			}
			start = i
		}
	}
	if start >= 0 {
		add(lines[start:])
	}
	hanging := append(tests, others...)
	if _, ok := firstUserFrame(parseFrames(hanging)); !ok {
		return lines
	}
	return hanging
}

// panicMessage returns the panic value: from "panic: " up to the stack,
// without the "[recovered]" marks the testing package adds.
func panicMessage(lines []string) string {
	var msg []string
	for i, l := range lines {
		if i > 0 && (strings.TrimSpace(l) == "" || strings.HasPrefix(l, "\tpanic: ") ||
			strings.HasPrefix(l, "[signal ") || strings.HasPrefix(l, "goroutine ")) {
			break
		}
		msg = append(msg, l)
	}
	return recoveredRE.ReplaceAllString(strings.Join(msg, "\n"), "")
}

func stackFinding(source, guide, test, msg string, after []string) finding {
	f := finding{
		source:   source,
		guide:    guide,
		raw:      Raw{Source: source, Test: test, Message: msg},
		builtins: map[string]string{"test": test},
	}
	if fr, ok := firstUserFrame(parseFrames(after)); ok {
		f.raw.File, f.raw.Line = path.Base(fr.path), fr.line
		f.builtins["func"] = shortFunc(fr.fn)
		f.builtins["file"] = f.raw.File
		f.builtins["line"] = itoa(fr.line)
	}
	return f
}

// raceFindings returns 1 finding per race report.
func raceFindings(test string, lines []string) []finding {
	var fs []finding
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "WARNING: DATA RACE" {
			continue
		}
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != raceSeparator {
			end++
		}
		fs = append(fs, raceFinding(contextTest(test, lines, i, -1), lines[i:end]))
		i = end
	}
	return fs
}

// raceFinding reads 1 report: the access that was caught, the previous
// access it conflicts with, and where both goroutines were created.
func raceFinding(test string, report []string) finding {
	f := finding{
		source:   SourceRace,
		guide:    GuideRaceReport,
		builtins: map[string]string{"test": test},
	}
	header := []string{strings.TrimSpace(report[0])}
	var sections [][]string
	for _, l := range report[1:] {
		if m := raceAccessRE.FindStringSubmatch(l); m != nil && len(sections) < 2 {
			header = append(header, l)
			prefix := ""
			if m[1] != "" {
				prefix = "prev_"
			}
			f.builtins[prefix+"access"] = strings.ToLower(m[2])
			sections = append(sections, nil)
			continue
		}
		if !strings.HasPrefix(l, " ") && strings.TrimSpace(l) != "" {
			// "Goroutine 7 (running) created at:" ends the access blocks.
			if len(sections) == 2 {
				sections = append(sections, nil)
			}
		}
		if n := len(sections); n > 0 && n <= 2 {
			sections[n-1] = append(sections[n-1], l)
		}
	}
	for i, prefix := range []string{"", "prev_"} {
		if i >= len(sections) {
			break
		}
		fr, ok := firstUserFrame(parseFrames(sections[i]))
		if !ok {
			continue
		}
		f.builtins[prefix+"func"] = shortFunc(fr.fn)
		f.builtins[prefix+"file"] = path.Base(fr.path)
		f.builtins[prefix+"line"] = itoa(fr.line)
	}
	f.raw = Raw{
		Source:  SourceRace,
		File:    f.builtins["file"],
		Line:    atoi(f.builtins["line"]),
		Test:    test,
		Message: strings.Join(header, "\n"),
	}
	if f.raw.File == "" {
		f.raw.File, f.raw.Line = f.builtins["prev_file"], atoi(f.builtins["prev_line"])
	}
	// The patterns run against the whole report, so an entry can match on
	// a frame (for example a map access); Raw keeps only the headline.
	f.text = strings.Join(report, "\n")
	return f
}

// exampleFindings finds an Example whose output differs: the testing
// package prints "got:" and "want:" blocks.
func exampleFindings(test string, lines []string) []finding {
	name := test
	isExample := strings.HasPrefix(test, "Example")
	for i, l := range lines {
		if m := failLineRE.FindStringSubmatch(l); m != nil && strings.HasPrefix(m[1], "Example") {
			isExample = true
			if name == "" || !strings.HasPrefix(name, "Example") {
				name = m[1]
			}
		}
		if !isExample || l != "got:" {
			continue
		}
		var block []string
		hasWant := false
		for _, r := range lines[i:] {
			if r == "want:" {
				hasWant = true
			} else if hasWant && (strings.HasPrefix(r, "FAIL") || strings.HasPrefix(r, "--- ") ||
				strings.HasPrefix(r, "=== ") || strings.HasPrefix(r, "ok ")) {
				break
			}
			block = append(block, r)
		}
		if !hasWant {
			continue
		}
		msg := strings.TrimRight(strings.Join(block, "\n"), "\n")
		return []finding{{
			source:   SourceTest,
			guide:    GuideFailingTest,
			raw:      Raw{Source: SourceTest, Test: name, Message: msg},
			builtins: map[string]string{"test": name},
		}}
	}
	return nil
}

// assertFindings returns 1 finding per "file_test.go:NN: message" line,
// or "file.go:NN: message" from a helper, with its more deeply indented
// continuation lines.
func assertFindings(test string, lines []string) []finding {
	var fs []finding
	for i := 0; i < len(lines); i++ {
		m := assertRE.FindStringSubmatch(lines[i])
		// The testing package reports its own problems from testing.go,
		// such as "race detected during execution of test"; the race
		// report itself is explained already.
		if m == nil || m[2] == "testing.go" {
			continue
		}
		indent := len(m[1])
		msg := []string{m[4]}
		j := i + 1
		for ; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" || assertRE.MatchString(l) || testLineRE.MatchString(l) ||
				indentOf(l) <= indent {
				break
			}
			msg = append(msg, strings.TrimSpace(l))
		}
		name := contextTest(test, lines, i, indent)
		fs = append(fs, finding{
			source: SourceTest,
			guide:  GuideFailingTest,
			raw: Raw{Source: SourceTest, File: m[2], Line: atoi(m[3]), Test: name,
				Message: strings.Join(msg, "\n")},
			builtins: map[string]string{"file": m[2], "line": m[3], "test": name},
		})
		i = j - 1
	}
	return fs
}

// contextTest names the test that printed line i: the caller's test name
// when there is one. In plain go test output the testing package prints a
// test's logs after its "--- FAIL:" line, indented 4 spaces deeper; with -v
// it prints them after the test's "=== RUN" line. indent is the
// indentation of line i, or -1 for a line that belongs to the nearest test
// above it (a panic).
func contextTest(test string, lines []string, i, indent int) string {
	if test != "" {
		return test
	}
	if indent >= 4 {
		for j := i - 1; j >= 0; j-- {
			// With -v the logs follow the test's "=== RUN" line.
			if m := runLineRE.FindStringSubmatch(lines[j]); m != nil {
				return m[1]
			}
			if m := failLineRE.FindStringSubmatch(lines[j]); m != nil && indentOf(lines[j]) == indent-4 {
				return m[1]
			}
		}
	}
	for j := i; j >= 0; j-- {
		if m := testLineRE.FindStringSubmatch(lines[j]); m != nil {
			return m[1]
		}
	}
	return ""
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
