package course

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kinds of Step.
const (
	StepKindStep = "step"
	StepKindPart = "part"
	StepKindAll  = "all"
)

// AllTestsID is the ID of the step that runs every test of an exercise.
const AllTestsID = "all"

// Step is a test filter from the README's "Run the tests" section, or the
// extra "All tests" step that runs everything.
type Step struct {
	ID      string `json:"id"`      // "step-1", "part-2" or AllTestsID; unique within an exercise
	N       int    `json:"n"`       // the number in the README; 0 for the "All tests" step
	Kind    string `json:"kind"`    // StepKindStep, StepKindPart or StepKindAll
	Label   string `json:"label"`   // the text after "step N:"; may be empty
	Run     string `json:"run"`     // the -run regexp for the runner; empty runs every test
	Race    bool   `json:"race"`    // the README runs this step with -race
	Command string `json:"command"` // the command as the README shows it
}

// Title is the name the UI shows, for example "Step 1: Parse" or "All tests".
func (s Step) Title() string {
	if s.Kind == StepKindAll {
		return "All tests"
	}
	t := fmt.Sprintf("%s %d", strings.ToUpper(s.Kind[:1])+s.Kind[1:], s.N)
	if s.Label != "" {
		t += ": " + s.Label
	}
	return t
}

// IsAll reports whether s is the "All tests" step.
func (s Step) IsAll() bool { return s.Kind == StepKindAll }

var stepComment = regexp.MustCompile(`^(?i:(step|part))\s+(\d+)\b\s*(.*)$`)

// parseSteps reads the step lines of a "Run the tests" section. A step line
// is a go test command with a comment "# step N: label" or "# part N: label";
// other commands (the full run, a single test, fuzzing) are examples for the
// terminal and not steps. tests are the top-level test names of the
// exercise; a -skip filter is resolved against them because the runner only
// takes -run.
func parseSteps(section, slug string, tests []string) ([]Step, error) {
	var steps []Step
	seen := map[string]bool{}
	for line := range strings.Lines(section) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "go test") {
			continue
		}
		args, comment, err := shellSplit(line)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", line, err)
		}
		m := stepComment.FindStringSubmatch(comment)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("%q: step number: %w", line, err)
		}
		st := Step{
			N:       n,
			Kind:    strings.ToLower(m[1]),
			Label:   cleanLabel(m[3]),
			Command: strings.TrimSpace(strings.SplitN(line, " #", 2)[0]),
		}
		st.ID = fmt.Sprintf("%s-%d", st.Kind, st.N)
		if seen[st.ID] {
			return nil, fmt.Errorf("%q: duplicate %s", line, st.ID)
		}
		seen[st.ID] = true
		if err := st.applyArgs(args, slug, tests); err != nil {
			return nil, fmt.Errorf("%q: %w", line, err)
		}
		steps = append(steps, st)
	}
	steps = append(steps, Step{
		ID:      AllTestsID,
		Kind:    StepKindAll,
		Label:   "All tests",
		Race:    true,
		Command: fmt.Sprintf("go test -race ./exercises/%s/", slug),
	})
	return steps, nil
}

// cleanLabel turns ": Parse", "(uses RequestID)" and "" into a label.
func cleanLabel(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ":"))
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func (st *Step) applyArgs(args []string, slug string, tests []string) error {
	if len(args) < 2 || args[0] != "go" || args[1] != "test" {
		return errors.New("not a go test command")
	}
	var skip string
	pkgSeen := false
	for i := 2; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		switch {
		case !strings.HasPrefix(a, "-"):
			if strings.TrimSuffix(a, "/") != "./exercises/"+slug {
				return fmt.Errorf("package %q is not ./exercises/%s/", a, slug)
			}
			pkgSeen = true
		case name == "race":
			st.Race = true
		case name == "v":
		case name == "run" || name == "skip" || name == "count":
			if !hasValue {
				if i+1 >= len(args) {
					return fmt.Errorf("flag %s needs a value", a)
				}
				i++
				value = args[i]
			}
			switch name {
			case "run":
				st.Run = value
			case "skip":
				skip = value
			}
			// -count only repeats the run; the runner always uses -count=1.
		default:
			return fmt.Errorf("unsupported flag %q", a)
		}
	}
	if !pkgSeen {
		return errors.New("no package path")
	}
	if skip != "" {
		run, err := resolveSkip(st.Run, skip, tests)
		if err != nil {
			return err
		}
		st.Run = run
	}
	return nil
}

// resolveSkip turns "-run R -skip S" into one -run regexp that lists the
// matching top-level tests by name.
func resolveSkip(run, skip string, tests []string) (string, error) {
	if strings.Contains(run, "/") || strings.Contains(skip, "/") {
		return "", errors.New("-skip with subtest patterns is not supported")
	}
	runMatch, err := TopLevelMatcher(run)
	if err != nil {
		return "", err
	}
	skipMatch, err := TopLevelMatcher(skip)
	if err != nil {
		return "", err
	}
	var keep []string
	for _, t := range tests {
		if runMatch(t) && !skipMatch(t) {
			keep = append(keep, regexp.QuoteMeta(t))
		}
	}
	if len(keep) == 0 {
		return "", fmt.Errorf("-run %q -skip %q selects no test", run, skip)
	}
	return "^(" + strings.Join(keep, "|") + ")$", nil
}

// TopLevelMatcher returns a func that reports whether `go test -run pattern`
// runs the top-level test, fuzz target or example name. Like the testing
// package it splits the pattern at unbracketed "|" and "/" and matches the
// first element of each alternative; the other elements select subtests.
func TopLevelMatcher(pattern string) (func(name string) bool, error) {
	if pattern == "" {
		return func(string) bool { return true }, nil
	}
	var res []*regexp.Regexp
	for _, alt := range splitRegexp(pattern) {
		re, err := regexp.Compile(alt[0])
		if err != nil {
			return nil, fmt.Errorf("-run %q: %w", pattern, err)
		}
		res = append(res, re)
		for _, sub := range alt[1:] {
			if _, err := regexp.Compile(sub); err != nil {
				return nil, fmt.Errorf("-run %q: %w", pattern, err)
			}
		}
	}
	return func(name string) bool {
		return slices.ContainsFunc(res, func(re *regexp.Regexp) bool { return re.MatchString(name) })
	}, nil
}

// splitRegexp mirrors testing.splitRegexp: alternatives at top-level "|",
// each a sequence of per-level patterns split at top-level "/".
func splitRegexp(s string) [][]string {
	var alts [][]string
	var cur []string
	brackets, parens := 0, 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '[':
			brackets++
		case ']':
			if brackets--; brackets < 0 {
				brackets = 0
			}
		case '(':
			if brackets == 0 {
				parens++
			}
		case ')':
			if brackets == 0 {
				parens--
			}
		case '\\':
			i++
		case '/', '|':
			if brackets == 0 && parens == 0 {
				cur = append(cur, s[:i])
				if s[i] == '|' {
					alts = append(alts, cur)
					cur = nil
				}
				s = s[i+1:]
				i = 0
				continue
			}
		}
		i++
	}
	cur = append(cur, s)
	return append(alts, cur)
}

// shellSplit splits a command line like a POSIX shell does for the simple
// commands in the READMEs: single and double quotes, backslash escapes and a
// trailing "# comment".
func shellSplit(line string) (args []string, comment string, err error) {
	var cur strings.Builder
	inToken := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			if inToken {
				args = append(args, cur.String())
				cur.Reset()
				inToken = false
			}
		case c == '#' && !inToken:
			return args, strings.TrimSpace(line[i+1:]), nil
		case c == '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, "", errors.New("unterminated single quote")
			}
			cur.WriteString(line[i+1 : i+1+end])
			i += end + 1
			inToken = true
		case c == '"':
			i++
			for ; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && strings.IndexByte(`"\$`+"`", line[i+1]) >= 0 {
					i++
				}
				cur.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, "", errors.New("unterminated double quote")
			}
			inToken = true
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			inToken = true
		default:
			cur.WriteByte(c)
			inToken = true
		}
	}
	if inToken {
		args = append(args, cur.String())
	}
	return args, "", nil
}

// testNames returns the top-level functions that `go test -run` can select
// in the given test files: tests, fuzz targets and examples. Benchmarks are
// left out because -run does not run them.
func testNames(files []File) ([]string, error) {
	fset := token.NewFileSet()
	var names []string
	for _, f := range files {
		af, err := parser.ParseFile(fset, filepath.Base(f.Name), f.Content, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.Name, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			n := fd.Name.Name
			if isTestFunc(n, "Test") || isTestFunc(n, "Fuzz") || strings.HasPrefix(n, "Example") {
				names = append(names, n)
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// isTestFunc applies the go test naming rule: the prefix, then nothing or a
// character that is not a lower-case letter.
func isTestFunc(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}
