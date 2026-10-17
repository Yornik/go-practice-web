package course

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

// courseDir is the submodule at the repository root.
const courseDir = "../../course"

var (
	loadOnce sync.Once
	loaded   *Course
	loadErr  error
)

// loadedOnce loads the real course once for all tests: they only read it.
func loadedOnce(t *testing.T) *Course {
	t.Helper()
	loadOnce.Do(func() { loaded, loadErr = Load(courseDir) })
	if loadErr != nil {
		t.Fatalf("Load(%s): %v", courseDir, loadErr)
	}
	return loaded
}

func TestLoadExercisesAndTiers(t *testing.T) {
	c := loadedOnce(t)
	if got := len(c.Exercises); got != 25 {
		t.Fatalf("got %d exercises, want 25", got)
	}
	for i, e := range c.Exercises {
		if e.Number != i+1 {
			t.Errorf("exercise %d has number %d", i, e.Number)
		}
		if !strings.HasPrefix(e.Slug, fmt.Sprintf("%02d-", e.Number)) {
			t.Errorf("slug %q does not start with its number", e.Slug)
		}
		if e.Title == "" || e.Time == "" || e.Hours <= 0 || e.Topics == "" || e.ProblemSolving == "" {
			t.Errorf("%s: incomplete header: %+v", e.Slug, e)
		}
		if got, ok := c.Exercise(e.Slug); !ok || got != e {
			t.Errorf("Exercise(%q) = %v, %v", e.Slug, got, ok)
		}
	}

	want := []struct{ first, last int }{{1, 3}, {4, 9}, {10, 14}, {15, 19}, {20, 25}}
	if len(c.Tiers) != len(want) {
		t.Fatalf("got %d tiers, want %d", len(c.Tiers), len(want))
	}
	for i, w := range want {
		tier, ok := c.Tier(i + 1)
		if !ok {
			t.Fatalf("Tier(%d) missing", i+1)
		}
		if tier.First != w.first || tier.Last != w.last || tier.Theme == "" {
			t.Errorf("tier %d = %d–%d %q, want %d–%d", tier.N, tier.First, tier.Last, tier.Theme, w.first, w.last)
		}
		for _, e := range tier.Exercises {
			if e.Tier != tier.N || e.Number < w.first || e.Number > w.last {
				t.Errorf("tier %d holds %s (tier %d)", tier.N, e.Slug, e.Tier)
			}
		}
		if len(tier.Exercises) != w.last-w.first+1 {
			t.Errorf("tier %d has %d exercises", tier.N, len(tier.Exercises))
		}
	}
	if _, ok := c.Tier(0); ok {
		t.Error("Tier(0) exists")
	}
	if _, ok := c.Tier(6); ok {
		t.Error("Tier(6) exists")
	}
}

func TestLookups(t *testing.T) {
	c := loadedOnce(t)
	e, ok := c.Exercise("07-retry")
	if !ok {
		t.Fatal("07-retry not found")
	}
	if e.Package != "retry" || e.Tier != 2 || e.Time != "2.5 h" || e.Title != "Retries with backoff, jitter and context" {
		t.Errorf("07-retry = %q %d %q %q", e.Package, e.Tier, e.Time, e.Title)
	}
	if n, ok := c.ExerciseByNumber(7); !ok || n != e {
		t.Error("ExerciseByNumber(7) is not 07-retry")
	}
	if _, ok := c.Exercise("../solutions"); ok {
		t.Error("found an exercise for a path")
	}
	prev, next := c.Neighbours("07-retry")
	if prev == nil || prev.Slug != "06-envconfig" || next == nil || next.Slug != "08-ratelimit" {
		t.Errorf("Neighbours(07-retry) = %v, %v", prev, next)
	}
	if prev, _ := c.Neighbours("01-logfmt"); prev != nil {
		t.Error("01-logfmt has a previous exercise")
	}
	if _, next := c.Neighbours("25-loadbalancer"); next != nil {
		t.Error("25-loadbalancer has a next exercise")
	}
	if got := c.Version(); got == "" {
		t.Error("empty Version")
	}
	if len(c.Digest) != 64 {
		t.Errorf("Digest = %q", c.Digest)
	}
}

func TestRootTableMatchesExerciseHeaders(t *testing.T) {
	c := loadedOnce(t)
	for _, e := range c.Exercises {
		if e.table.title != e.Title || e.table.tier != fmt.Sprint(e.Tier) || e.table.time != e.Time {
			t.Errorf("%s: root table %+v, README %q tier %d %q", e.Slug, e.table, e.Title, e.Tier, e.Time)
		}
		if e.Practice == "" || e.Skill == "" {
			t.Errorf("%s: no Practice or Skill from the root table", e.Slug)
		}
	}
}

func TestSteps(t *testing.T) {
	c := loadedOnce(t)
	for _, e := range c.Exercises {
		t.Run(e.Slug, func(t *testing.T) {
			if len(e.Steps) < 3 {
				t.Fatalf("got %d steps (with All tests), want at least 2 README steps", len(e.Steps))
			}
			all := e.AllTests()
			if all.ID != AllTestsID || all.Run != "" || !all.Race || all.Title() != "All tests" {
				t.Errorf("last step = %+v, want All tests with -race", all)
			}
			filtered := 0
			ids := map[string]bool{}
			for _, s := range e.Steps {
				if ids[s.ID] {
					t.Errorf("duplicate step ID %q", s.ID)
				}
				ids[s.ID] = true
				if got, ok := e.Step(s.ID); !ok || got != s {
					t.Errorf("Step(%q) = %+v, %v", s.ID, got, ok)
				}
				if s.Run == "" {
					continue
				}
				match, err := TopLevelMatcher(s.Run)
				if err != nil {
					t.Errorf("%s: %v", s.Title(), err)
					continue
				}
				if !slices.ContainsFunc(e.Tests, match) {
					t.Errorf("%s: -run %q matches none of %v", s.Title(), s.Run, e.Tests)
				}
				filtered++
			}
			if filtered < 2 {
				t.Errorf("got %d steps with a -run filter, want at least 2", filtered)
			}
		})
	}
}

func TestStepDetails(t *testing.T) {
	c := loadedOnce(t)
	tests := []struct {
		slug string
		id   string
		want Step
	}{
		{"01-logfmt", "step-1", Step{ID: "step-1", N: 1, Kind: "step", Label: "Parse", Run: "TestParse|TestSyntaxError|FuzzParse",
			Command: "go test -run 'TestParse|TestSyntaxError|FuzzParse' -v ./exercises/01-logfmt/"}},
		{"05-slo", "step-1", Step{ID: "step-1", N: 1, Kind: "step", Run: "TestValidate",
			Command: "go test -run TestValidate -v ./exercises/05-slo/"}},
		{"07-retry", "step-4", Step{ID: "step-4", N: 4, Kind: "step", Label: "Do and DoValue", Run: "TestDo", Race: true,
			Command: "go test -race -run TestDo -v ./exercises/07-retry/"}},
		{"10-deployorder", "step-6", Step{ID: "step-6", N: 6, Kind: "step", Label: "all, with Example and TestConcurrentUse", Race: true,
			Command: "go test -race ./exercises/10-deployorder/"}},
		{"12-workerpool", "part-2", Step{ID: "part-2", N: 2, Kind: "part", Label: "Group", Run: "TestGroup", Race: true,
			Command: "go test -race -run TestGroup -v ./exercises/12-workerpool/"}},
		{"16-middleware", "step-5", Step{ID: "step-5", N: 5, Kind: "step", Label: "uses RequestID", Run: "TestAccessLog", Race: true,
			Command: "go test -race -run TestAccessLog ./exercises/16-middleware/"}},
	}
	for _, tt := range tests {
		e, _ := c.Exercise(tt.slug)
		got, ok := e.Step(tt.id)
		if !ok {
			t.Errorf("%s: no step %q", tt.slug, tt.id)
			continue
		}
		if got != tt.want {
			t.Errorf("%s %s:\n got %+v\nwant %+v", tt.slug, tt.id, got, tt.want)
		}
	}

	// The README's -skip becomes an explicit list: the runner takes -run only.
	e, _ := c.Exercise("18-certcheck")
	s, _ := e.Step("step-1")
	match, err := TopLevelMatcher(s.Run)
	if err != nil {
		t.Fatal(err)
	}
	if match("TestCheckAll") || !match("TestCheck") && !slices.ContainsFunc(e.Tests, match) {
		t.Errorf("18-certcheck step 1 run %q", s.Run)
	}
	for _, name := range e.Tests {
		want := strings.HasPrefix(name, "TestCheck") && !strings.HasPrefix(name, "TestCheckAll")
		if match(name) != want {
			t.Errorf("18-certcheck step 1 run %q: match(%s) = %v, want %v", s.Run, name, match(name), want)
		}
	}
}

func TestParseSteps(t *testing.T) {
	tests := []string{"TestA", "TestAB", "TestB", "ExampleA"}
	section := `
    go test ./exercises/99-x/
    go test -run TestA -v ./exercises/99-x/                  # not a step comment
    go test -run "Test(A|B)$" -v ./exercises/99-x/           # Step 1: quoted
    go test -race -run=TestA -skip=TestAB ./exercises/99-x/  # part 2
    go test -count=5 -run TestB ./exercises/99-x            # step 3 (counted)
`
	got, err := parseSteps(section, "99-x", tests)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{ID: "step-1", N: 1, Kind: "step", Label: "quoted", Run: "Test(A|B)$", Command: `go test -run "Test(A|B)$" -v ./exercises/99-x/`},
		{ID: "part-2", N: 2, Kind: "part", Run: "^(TestA)$", Race: true, Command: "go test -race -run=TestA -skip=TestAB ./exercises/99-x/"},
		{ID: "step-3", N: 3, Kind: "step", Label: "counted", Run: "TestB", Command: "go test -count=5 -run TestB ./exercises/99-x"},
		{ID: "all", Kind: "all", Label: "All tests", Race: true, Command: "go test -race ./exercises/99-x/"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseSteps:\n got %+v\nwant %+v", got, want)
	}

	bad := map[string]string{
		"wrong package":  "go test -run TestA ./exercises/01-logfmt/ # step 1: x",
		"unknown flag":   "go test -fuzz=FuzzA ./exercises/99-x/ # step 1: x",
		"duplicate":      "go test -run TestA ./exercises/99-x/ # step 1\ngo test -run TestB ./exercises/99-x/ # step 1",
		"open quote":     "go test -run 'TestA ./exercises/99-x/ # step 1",
		"skip all":       "go test -run TestA -skip TestA ./exercises/99-x/ # step 1",
		"skip subtests":  "go test -run TestA/x -skip TestAB ./exercises/99-x/ # step 1",
		"missing value":  "go test ./exercises/99-x/ -run # step 1",
		"no package":     "go test -run TestA # step 1",
		"bad run regexp": "go test -run 'Test(' -skip X ./exercises/99-x/ # step 1",
	}
	for name, src := range bad {
		if _, err := parseSteps(src, "99-x", tests); err == nil {
			t.Errorf("%s: parseSteps(%q) succeeded", name, src)
		}
	}
}

func TestTopLevelMatcher(t *testing.T) {
	tests := []struct {
		pattern string
		match   []string
		noMatch []string
	}{
		{"", []string{"TestA"}, nil},
		{"TestA", []string{"TestA", "TestAB"}, []string{"TestB"}},
		{"TestCriticalPath$/overlaps", []string{"TestCriticalPath"}, []string{"TestCriticalPathX"}},
		{"TestA/x|TestB", []string{"TestA", "TestB"}, []string{"TestC"}},
		{"Test(A|B)$", []string{"TestA", "TestB"}, []string{"TestAB"}},
		{"Test[/|]", []string{"Test/", "Test|"}, []string{"TestA"}},
	}
	for _, tt := range tests {
		m, err := TopLevelMatcher(tt.pattern)
		if err != nil {
			t.Errorf("TopLevelMatcher(%q): %v", tt.pattern, err)
			continue
		}
		for _, n := range tt.match {
			if !m(n) {
				t.Errorf("%q does not match %q", tt.pattern, n)
			}
		}
		for _, n := range tt.noMatch {
			if m(n) {
				t.Errorf("%q matches %q", tt.pattern, n)
			}
		}
	}
	if _, err := TopLevelMatcher("Test("); err == nil {
		t.Error("TopLevelMatcher(\"Test(\") succeeded")
	}
	if _, err := TopLevelMatcher("TestA/x("); err == nil {
		t.Error("TopLevelMatcher with a bad subtest pattern succeeded")
	}
}

func TestShellSplit(t *testing.T) {
	tests := []struct {
		line    string
		args    []string
		comment string
	}{
		{`go test -run 'A|B' ./x/  # step 1: y`, []string{"go", "test", "-run", "A|B", "./x/"}, "step 1: y"},
		{`go test -run "A\"B" x`, []string{"go", "test", "-run", `A"B`, "x"}, ""},
		{`go test -run A#B x # c`, []string{"go", "test", "-run", "A#B", "x"}, "c"},
		{`go test a\ b`, []string{"go", "test", "a b"}, ""},
	}
	for _, tt := range tests {
		args, comment, err := shellSplit(tt.line)
		if err != nil || !slices.Equal(args, tt.args) || comment != tt.comment {
			t.Errorf("shellSplit(%q) = %q, %q, %v; want %q, %q", tt.line, args, comment, err, tt.args, tt.comment)
		}
	}
	for _, line := range []string{`go test 'x`, `go test "x`} {
		if _, _, err := shellSplit(line); err == nil {
			t.Errorf("shellSplit(%q) succeeded", line)
		}
	}
}

func TestFiles(t *testing.T) {
	c := loadedOnce(t)
	for _, e := range c.Exercises {
		stubs, tests := e.StubFiles(), e.TestFiles()
		if len(stubs) == 0 || len(tests) == 0 || len(stubs)+len(tests) != len(e.Files) {
			t.Errorf("%s: %d stubs, %d tests, %d files", e.Slug, len(stubs), len(tests), len(e.Files))
		}
		for _, f := range stubs {
			if f.ReadOnly || strings.HasSuffix(f.Name, "_test.go") || !e.IsStub(f.Name) {
				t.Errorf("%s: bad stub %s", e.Slug, f.Name)
			}
			if !strings.Contains(f.Content, `panic("TODO: implement")`) {
				t.Errorf("%s: stub %s does not panic with TODO", e.Slug, f.Name)
			}
			if !strings.Contains(f.Content, "package "+e.Package) {
				t.Errorf("%s: stub %s is not package %s", e.Slug, f.Name, e.Package)
			}
		}
		for _, f := range tests {
			if !f.ReadOnly || !strings.HasSuffix(f.Name, "_test.go") || e.IsStub(f.Name) || f.Content == "" {
				t.Errorf("%s: bad test file %s", e.Slug, f.Name)
			}
		}
		if !slices.IsSorted(e.Tests) || len(e.Tests) == 0 {
			t.Errorf("%s: tests %v", e.Slug, e.Tests)
		}
		for _, name := range e.StubNames() {
			if _, ok := e.File(name); !ok {
				t.Errorf("%s: File(%q) missing", e.Slug, name)
			}
		}
	}
	e, _ := c.Exercise("17-promtext")
	if len(e.Testdata) == 0 {
		t.Error("17-promtext lists no testdata")
	}
	for _, name := range e.Testdata {
		if strings.Contains(name, `\`) || strings.HasPrefix(name, "/") {
			t.Errorf("testdata name %q", name)
		}
	}
	if _, ok := e.File("../../go.mod"); ok {
		t.Error("File accepts a path")
	}
}

func TestSections(t *testing.T) {
	c := loadedOnce(t)
	want := []string{SectionTask, SectionRules, SectionExamples, SectionRunTests, SectionDocs, SectionStretch, SectionStuck}
	for _, e := range c.Exercises {
		var titles []string
		for _, s := range e.Sections {
			titles = append(titles, s.Title)
			if s.ID == "" || s.HTML == "" {
				t.Errorf("%s: section %q has no ID or body", e.Slug, s.Title)
			}
			if strings.Contains(string(s.HTML), "<h2") {
				t.Errorf("%s: section %q holds another h2", e.Slug, s.Title)
			}
		}
		if !slices.Equal(titles, want) {
			t.Errorf("%s: sections %q, want %q", e.Slug, titles, want)
		}
		if slices.ContainsFunc(e.SpecSections(), func(s Section) bool { return s.Title == SectionStuck }) {
			t.Errorf("%s: SpecSections has Stuck?", e.Slug)
		}
		if s, ok := e.Section(SectionRunTests); !ok || s.ID != "run-the-tests" {
			t.Errorf("%s: Section(Run the tests) = %+v, %v", e.Slug, s, ok)
		}
		if strings.Contains(string(e.Intro), "Tier ") && strings.Contains(string(e.Intro), "Problem solving") {
			t.Errorf("%s: Intro holds the tier line", e.Slug)
		}
		if strings.Contains(string(e.Intro), "<h1") || e.Intro == "" {
			t.Errorf("%s: Intro = %.80q", e.Slug, e.Intro)
		}
	}
}

var (
	// A table that goldmark did not render leaves its separator row behind.
	// Code blocks may draw ASCII diagrams, so they are cut out first.
	tablePipes = regexp.MustCompile(`\|\s*:?-{3,}`)
	preBlocks  = regexp.MustCompile(`(?s)<pre.*?</pre>`)
	// ROT13 of common English and Go words: "the", "and", "package", "func".
	rot13Words = regexp.MustCompile(`\b(gur|naq|cnpxntr|shap)\b`)
)

func TestRenderedHTML(t *testing.T) {
	c := loadedOnce(t)
	check := func(t *testing.T, name string, html string) {
		t.Helper()
		if html == "" {
			t.Errorf("%s: empty", name)
		}
		text := preBlocks.ReplaceAllString(html, "")
		if loc := tablePipes.FindStringIndex(text); loc != nil {
			t.Errorf("%s: unrendered table near %q", name, text[max(0, loc[0]-40):min(len(text), loc[1]+40)])
		}
		if n := len(rot13Words.FindAllString(html, -1)); n > 0 {
			t.Errorf("%s: %d ROT13 words", name, n)
		}
		if strings.Contains(html, "raw HTML omitted") {
			t.Errorf("%s: holds raw HTML", name)
		}
	}
	for _, s := range c.Guide {
		check(t, "guide "+s.Title, string(s.HTML))
	}
	check(t, "intro", string(c.Intro))
	for _, e := range c.Exercises {
		// In parallel: the hidden material renders on first use, and
		// concurrent reveals must be safe.
		t.Run(e.Slug, func(t *testing.T) {
			t.Parallel()
			check(t, "README", string(e.HTML))
			if !strings.Contains(string(e.HTML), `class="chroma"`) {
				t.Error("README has no highlighted code")
			}
			for l := Hint1; l <= MaxLevel; l++ {
				h, err := e.Hidden(l)
				if err != nil {
					t.Fatalf("Hidden(%v): %v", l, err)
				}
				check(t, l.String(), string(h))
				if l.IsHint() && strings.Contains(string(h), "Hint ") && strings.Contains(string(h), "<h2") {
					t.Errorf("%v holds a hint heading", l)
				}
			}
			sol, _ := e.Hidden(Solution)
			for _, name := range e.StubNames() {
				if !strings.Contains(string(sol), ">"+name+"</h3>") {
					t.Errorf("solution HTML has no %s", name)
				}
			}
			if again, _ := e.Hidden(Solution); again != sol {
				t.Error("Hidden(Solution) changed between calls")
			}
		})
	}
}

func TestHidden(t *testing.T) {
	c := loadedOnce(t)
	for _, e := range c.Exercises {
		sol := e.SolutionFiles()
		var names []string
		for _, f := range sol {
			names = append(names, f.Name)
			if !strings.Contains(f.Content, "package "+e.Package) {
				t.Errorf("%s: solution %s is not decoded Go of package %s", e.Slug, f.Name, e.Package)
			}
			if strings.Contains(f.Content, `panic("TODO: implement")`) {
				t.Errorf("%s: solution %s is a stub", e.Slug, f.Name)
			}
		}
		if !slices.Equal(names, e.StubNames()) {
			t.Errorf("%s: solution files %v, stub files %v", e.Slug, names, e.StubNames())
		}
		hints := map[string]bool{}
		for l := Hint1; l <= Hint3; l++ {
			h, _ := e.Hidden(l)
			hints[string(h)] = true
		}
		if len(hints) != 3 {
			t.Errorf("%s: the 3 hints are not distinct", e.Slug)
		}
	}

	e := c.Exercises[0]
	for _, l := range []Level{0, -1, MaxLevel + 1} {
		if _, err := e.Hidden(l); err == nil {
			t.Errorf("Hidden(%d) succeeded", l)
		}
	}
	// SolutionFiles hands out a copy.
	e.SolutionFiles()[0].Content = "changed"
	if e.SolutionFiles()[0].Content == "changed" {
		t.Error("SolutionFiles shares its slice")
	}
}

func TestLevel(t *testing.T) {
	tests := []struct {
		l     Level
		name  string
		valid bool
		hint  bool
	}{
		{0, "Level(0)", false, false},
		{Hint1, "Hint 1", true, true},
		{Hint3, "Hint 3", true, true},
		{Explanation, "Explanation", true, false},
		{Walkthrough, "Walkthrough", true, false},
		{Solution, "Solution", true, false},
		{7, "Level(7)", false, false},
	}
	for _, tt := range tests {
		if tt.l.String() != tt.name || tt.l.Valid() != tt.valid || tt.l.IsHint() != tt.hint {
			t.Errorf("Level(%d): %q %v %v", int(tt.l), tt.l.String(), tt.l.Valid(), tt.l.IsHint())
		}
	}
}

func TestSplitHints(t *testing.T) {
	good := "# Title\n\n## Hint 1\n\none\n\n## Hint 2\n\ntwo\n## Hint 3\nthree\n"
	got, err := splitHints([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if string(got[0]) != "one" || string(got[1]) != "two" || string(got[2]) != "three" {
		t.Errorf("splitHints = %q", got)
	}
	for _, bad := range []string{
		"## Hint 1\na\n## Hint 2\nb\n",
		"## Hint 1\na\n## Hint 3\nb\n## Hint 2\nc\n",
		"## Hint 1\n\n## Hint 2\nb\n## Hint 3\nc\n",
		"## Hint 1\na\n## Hint 2\nb\n## Hint 3\nc\n## Hint 4\nd\n",
	} {
		if _, err := splitHints([]byte(bad)); err == nil {
			t.Errorf("splitHints(%q) succeeded", bad)
		}
	}
}

func TestGuide(t *testing.T) {
	c := loadedOnce(t)
	var titles []string
	for _, s := range c.Guide {
		titles = append(titles, s.Title)
	}
	if !slices.Equal(titles, GuideSections) {
		t.Errorf("guide sections %q, want %q", titles, GuideSections)
	}
	if !strings.Contains(string(c.Intro), "25 Go exercises") {
		t.Errorf("Intro = %.100q", c.Intro)
	}
}

func TestLinkRewrite(t *testing.T) {
	r := &linkRewriter{slugs: map[string]bool{"07-retry": true, "01-logfmt": true}}
	tests := []struct {
		base, dest string
		image      bool
		want       string
	}{
		{"README.md", "exercises/07-retry/", false, "/learn/07-retry"},
		{"README.md", "exercises/07-retry/README.md#rules", false, "/learn/07-retry#rules"},
		{"exercises/01-logfmt/README.md", "../07-retry/", false, "/learn/07-retry"},
		{"solutions/01-logfmt/hints.md", "../../exercises/07-retry", false, "/learn/07-retry"},
		{"README.md", "exercises/99-unknown/", false, GitHubBlobURL + "exercises/99-unknown"},
		{"README.md", "LICENSE", false, GitHubBlobURL + "LICENSE"},
		{"exercises/07-retry/README.md", "retry_test.go", false, GitHubBlobURL + "exercises/07-retry/retry_test.go"},
		{"exercises/07-retry/README.md", "/scripts/verify-solutions.sh", false, GitHubBlobURL + "scripts/verify-solutions.sh"},
		{"README.md", "../../etc/passwd", false, GitHubBlobURL},
		{"README.md", "https://go.dev/tour/", false, "https://go.dev/tour/"},
		{"README.md", "mailto:a@example.com", false, "mailto:a@example.com"},
		{"README.md", "#the-rules", false, "#the-rules"},
		{"README.md", "docs/a.png", true, "https://github.com/Yornik/go-practice/raw/main/docs/a.png"},
	}
	for _, tt := range tests {
		if got := r.rewrite(tt.base, tt.dest, tt.image); got != tt.want {
			t.Errorf("rewrite(%q, %q) = %q, want %q", tt.base, tt.dest, got, tt.want)
		}
	}

	md := newMarkdown(map[string]bool{"07-retry": true})
	h, err := md.render([]byte("See [retry](../07-retry/) and [the license](../../LICENSE), a <Field>.\n\n<script>x</script>\n"), "exercises/01-logfmt/README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="/learn/07-retry"`, `href="` + GitHubBlobURL + `LICENSE"`, "&lt;script&gt;x&lt;/script&gt;"} {
		if !strings.Contains(string(h), want) {
			t.Errorf("render: %q lacks %q", h, want)
		}
	}
	if !strings.Contains(string(h), "a &lt;Field&gt;.") {
		t.Errorf("render dropped inline HTML: %q", h)
	}
	if strings.Contains(string(h), "<script>") {
		t.Errorf("render let raw HTML through: %q", h)
	}
	// The real root README links an exercise to its page.
	root, err := os.ReadFile(filepath.Join(courseDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	full, err := newMarkdown(map[string]bool{"07-retry": true}).render(root, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(full), `href="/learn/07-retry"`) {
		t.Error("root README does not link 07-retry to /learn/07-retry")
	}
}

func TestHighlightCSS(t *testing.T) {
	css := HighlightCSS()
	for _, want := range []string{
		`[data-theme="light"] .chroma .k, :root:not([data-theme]) .chroma .k {`,
		`[data-theme="dark"] .chroma .k {`,
		"@media (prefers-color-scheme: dark) {",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("HighlightCSS lacks %q", want)
		}
	}
	for line := range strings.Lines(css) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "/*") || line == "}" || strings.HasPrefix(line, "@media") {
			continue
		}
		if !strings.HasPrefix(line, "[data-theme=") && !strings.HasPrefix(line, ":root:not([data-theme])") {
			t.Errorf("unscoped rule %q", line)
		}
	}
	if _, err := HighlightCSSFor("no-such-style", "github-dark"); err == nil {
		t.Error("HighlightCSSFor accepted an unknown style")
	}
}

func TestReadCommit(t *testing.T) {
	const hash = "1ff166840a4c5f62c96fb7a15891a84a248318da"
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"no git", nil, ""},
		{"detached", map[string]string{".git/HEAD": hash + "\n"}, hash},
		{"loose ref", map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".git/refs/heads/main": hash + "\n"}, hash},
		{"packed ref", map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".git/packed-refs": "# pack-refs\n" + hash + " refs/heads/main\n"}, hash},
		{"submodule", map[string]string{".git": "gitdir: ../meta\n", "../meta/HEAD": hash}, hash},
		{"garbage", map[string]string{".git/HEAD": "not a hash"}, ""},
		{"missing ref", map[string]string{".git/HEAD": "ref: refs/heads/gone\n"}, ""},
	}
	for _, tt := range tests {
		root := t.TempDir()
		dir := filepath.Join(root, "course")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range tt.files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := readCommit(dir); got != tt.want {
			t.Errorf("%s: readCommit = %q, want %q", tt.name, got, tt.want)
		}
	}
	// The submodule checkout in this repository resolves.
	if got := readCommit(courseDir); got != "" && !commitHash.MatchString(got) {
		t.Errorf("readCommit(course) = %q", got)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("Load of an empty directory succeeded")
	}
	// A copy of 1 exercise with a broken tier line fails with its slug.
	dir := t.TempDir()
	copyTree(t, filepath.Join(courseDir, "exercises", "01-logfmt"), filepath.Join(dir, "exercises", "01-logfmt"))
	copyTree(t, filepath.Join(courseDir, "solutions", "01-logfmt"), filepath.Join(dir, "solutions", "01-logfmt"))
	readme := filepath.Join(dir, "exercises", "01-logfmt", "README.md")
	b, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte(strings.Replace(string(b), "**Tier 1", "**Tier one", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "01-logfmt") {
		t.Errorf("Load with a broken tier line: %v", err)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
}
