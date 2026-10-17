package course

import (
	"fmt"
	"go/parser"
	"go/token"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// Section titles of an exercise README.
const (
	SectionTask     = "Task"
	SectionRules    = "Rules"
	SectionExamples = "Examples"
	SectionRunTests = "Run the tests"
	SectionDocs     = "Docs you can use"
	SectionStretch  = "Stretch goals"
	SectionStuck    = "Stuck?"
)

// FileKind tells stub files (the learner edits them) from test files.
type FileKind string

// Kinds of File.
const (
	FileStub FileKind = "stub"
	FileTest FileKind = "test"
)

// File is a Go file of an exercise.
type File struct {
	Name     string   `json:"name"`
	Kind     FileKind `json:"kind"`
	ReadOnly bool     `json:"readonly"`
	Content  string   `json:"content"`
}

// Exercise is 1 exercise of the course. It is read only after Load.
type Exercise struct {
	Number  int     // 7
	Slug    string  // "07-retry", the directory name
	Package string  // "retry"
	Title   string  // "Retries with backoff, jitter and context"
	Tier    int     // 1 to 5
	Hours   float64 // the time from the README, 2.5
	Time    string  // the same as text, "2.5 h"

	Topics         template.HTML // "Go: ..." from line 3 of the README
	ProblemSolving template.HTML // "Problem solving: ..." from line 3
	Practice       template.HTML // "Go you practice" from the course's exercise table
	Skill          template.HTML // "The real-life skill" from the course's exercise table

	Intro    template.HTML // the scenario between the header and the first section
	HTML     template.HTML // the whole README
	Sections []Section     // the README's "## " sections, in order

	Steps    []Step   // the README's steps, then the "All tests" step
	Files    []File   // the stub files, then the test files, each sorted by name
	Tests    []string // the top-level tests, fuzz targets and examples, sorted
	Testdata []string // file paths below testdata/, slash separated, sorted

	hidden *hidden
	table  tableRow // the root README's row, kept for consistency tests
}

// tableRow is an exercise's row in the root README's exercise table.
type tableRow struct{ title, tier, time string }

// Section returns the README section with the given title.
func (e *Exercise) Section(title string) (Section, bool) {
	for _, s := range e.Sections {
		if s.Title == title {
			return s, true
		}
	}
	return Section{}, false
}

// SpecSections returns the sections the IDE shows: every section except
// "Stuck?", whose peek commands the hint ladder replaces.
func (e *Exercise) SpecSections() []Section {
	out := make([]Section, 0, len(e.Sections))
	for _, s := range e.Sections {
		if s.Title != SectionStuck {
			out = append(out, s)
		}
	}
	return out
}

// Step returns the step with the given ID.
func (e *Exercise) Step(id string) (Step, bool) {
	i := slices.IndexFunc(e.Steps, func(s Step) bool { return s.ID == id })
	if i < 0 {
		return Step{}, false
	}
	return e.Steps[i], true
}

// AllTests returns the step that runs every test with -race. Passing it
// marks the exercise as passed.
func (e *Exercise) AllTests() Step {
	return e.Steps[len(e.Steps)-1]
}

// File returns the stub or test file with the given name.
func (e *Exercise) File(name string) (File, bool) {
	i := slices.IndexFunc(e.Files, func(f File) bool { return f.Name == name })
	if i < 0 {
		return File{}, false
	}
	return e.Files[i], true
}

// StubFiles returns the files the learner edits.
func (e *Exercise) StubFiles() []File {
	return slices.DeleteFunc(slices.Clone(e.Files), func(f File) bool { return f.Kind != FileStub })
}

// TestFiles returns the read-only test files.
func (e *Exercise) TestFiles() []File {
	return slices.DeleteFunc(slices.Clone(e.Files), func(f File) bool { return f.Kind != FileTest })
}

// StubNames returns the names of the stub files. The runner accepts exactly
// these names.
func (e *Exercise) StubNames() []string {
	var names []string
	for _, f := range e.Files {
		if f.Kind == FileStub {
			names = append(names, f.Name)
		}
	}
	return names
}

// IsStub reports whether name is 1 of the exercise's stub files.
func (e *Exercise) IsStub(name string) bool {
	f, ok := e.File(name)
	return ok && f.Kind == FileStub
}

// Hidden returns the rendered HTML of a level of the hint ladder. It is the
// only way to the hidden material: call it only after the learner revealed
// the level.
func (e *Exercise) Hidden(l Level) (template.HTML, error) {
	if !l.Valid() {
		return "", fmt.Errorf("%w: %d", ErrLevel, int(l))
	}
	return e.hidden.render(l)
}

// SolutionFiles returns the reference solution as plain Go files, named
// like the stubs. It is for tests and tools on the server; a browser gets
// the solution only through Hidden(Solution).
func (e *Exercise) SolutionFiles() []File {
	return slices.Clone(e.hidden.solution)
}

var (
	titleLine = regexp.MustCompile(`^# ([0-9]{2}) · (.+)$`)
	tierLine  = regexp.MustCompile(`^\*\*Tier ([1-5]) · about ([0-9]+(?:\.[0-9]+)?) h\*\* · Go: (.+?) · Problem solving: (.+)$`)
)

// loadExercise reads exercises/<slug>/ and solutions/<slug>/.
func loadExercise(md *markdown, dir, slug string) (*Exercise, error) {
	edir := filepath.Join(dir, "exercises", slug)
	readme, err := os.ReadFile(filepath.Join(edir, "README.md"))
	if err != nil {
		return nil, err
	}
	e := &Exercise{Slug: slug}
	if err := e.parseHeader(md, readme); err != nil {
		return nil, err
	}
	if want := fmt.Sprintf("%02d-", e.Number); !strings.HasPrefix(slug, want) {
		return nil, fmt.Errorf("README title number %d does not match the directory", e.Number)
	}
	if err := e.loadFiles(edir); err != nil {
		return nil, err
	}

	doc := md.parse(readme, "exercises/"+slug+"/README.md")
	parts := split(doc)
	if e.Intro, err = md.renderNodes(doc, introNodes(parts.preamble)); err != nil {
		return nil, err
	}
	// The sections and the whole README come from 1 rendering, because
	// highlighting the code blocks is most of the load time.
	if e.HTML, e.Sections, err = md.renderSplit(doc, parts); err != nil {
		return nil, err
	}
	i := slices.IndexFunc(parts.sections, func(s rawSection) bool { return s.title == SectionRunTests })
	if i < 0 {
		return nil, fmt.Errorf("README has no %q section", SectionRunTests)
	}
	if e.Steps, err = parseSteps(sectionSource(doc, parts.sections[i]), slug, e.Tests); err != nil {
		return nil, fmt.Errorf("steps: %w", err)
	}

	if e.hidden, err = loadHidden(md, dir, slug, e.StubNames()); err != nil {
		return nil, fmt.Errorf("hidden material: %w", err)
	}
	return e, nil
}

// introNodes drops the title and the tier line from the preamble: the UI
// shows them from the parsed fields.
func introNodes(preamble []ast.Node) []ast.Node {
	var out []ast.Node
	tierSkipped := false
	for _, n := range preamble {
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			continue
		}
		if _, ok := n.(*ast.Paragraph); ok && !tierSkipped {
			tierSkipped = true
			continue
		}
		out = append(out, n)
	}
	return out
}

// parseHeader reads line 1 ("# NN · Title") and line 3 (tier, time, topics).
func (e *Exercise) parseHeader(md *markdown, readme []byte) error {
	lines := strings.SplitN(string(readme), "\n", 4)
	if len(lines) < 3 {
		return fmt.Errorf("README has %d lines", len(lines))
	}
	m := titleLine.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if m == nil {
		return fmt.Errorf("README line 1 %q is not \"# NN · Title\"", lines[0])
	}
	e.Number, _ = strconv.Atoi(m[1])
	e.Title = m[2]

	m = tierLine.FindStringSubmatch(strings.TrimSpace(lines[2]))
	if m == nil {
		return fmt.Errorf("README line 3 is not \"**Tier N · about X h** · Go: ... · Problem solving: ...\"")
	}
	e.Tier, _ = strconv.Atoi(m[1])
	var err error
	if e.Hours, err = strconv.ParseFloat(m[2], 64); err != nil {
		return fmt.Errorf("README line 3: time %q: %w", m[2], err)
	}
	e.Time = m[2] + " h"
	if e.Topics, err = md.renderInline(m[3]); err != nil {
		return err
	}
	if e.ProblemSolving, err = md.renderInline(m[4]); err != nil {
		return err
	}
	return nil
}

// loadFiles reads the Go files and lists testdata/.
func (e *Exercise) loadFiles(edir string) error {
	entries, err := os.ReadDir(edir)
	if err != nil {
		return err
	}
	var stubs, tests []File
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(edir, name))
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, File{Name: name, Kind: FileTest, ReadOnly: true, Content: string(b)})
		} else {
			stubs = append(stubs, File{Name: name, Kind: FileStub, Content: string(b)})
		}
	}
	if len(stubs) == 0 || len(tests) == 0 {
		return fmt.Errorf("want stub and test files, found %d and %d", len(stubs), len(tests))
	}
	e.Files = append(stubs, tests...)

	if e.Package, err = packageName(stubs[0]); err != nil {
		return err
	}
	if e.Tests, err = testNames(tests); err != nil {
		return err
	}

	td := filepath.Join(edir, "testdata")
	err = filepath.WalkDir(td, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(td, p)
			if err != nil {
				return err
			}
			e.Testdata = append(e.Testdata, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list testdata: %w", err)
	}
	slices.Sort(e.Testdata)
	return nil
}

func packageName(f File) (string, error) {
	af, err := parser.ParseFile(token.NewFileSet(), f.Name, f.Content, parser.PackageClauseOnly)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", f.Name, err)
	}
	return af.Name.Name, nil
}
