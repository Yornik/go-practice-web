package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// exerciseDirRE matches exercise directory names such as "32-retry". The
// names are matched against the index, never joined into paths from input.
var exerciseDirRE = regexp.MustCompile(`^[0-9]{2}-[a-z0-9]+(-[a-z0-9]+)*$`)

// Course is the index of the course checkout that the runner needs: per
// exercise, which files the learner may send and which files come from the
// course. It is built once at start and read-only afterwards.
type Course struct {
	dir       string
	goMod     []byte
	exercises map[string]*Exercise
}

// Exercise describes one exercises/NN-slug directory.
type Exercise struct {
	// Name is the directory name, for example "32-retry".
	Name string
	// Dir is the absolute path of the exercise in the course checkout.
	Dir string
	// Stubs are the names of the non-test .go files: the only names a
	// request may send. Sorted.
	Stubs []string
	// Tests are the names of the _test.go files. Sorted.
	Tests []string
	// HasTestdata reports whether the exercise has a testdata directory.
	HasTestdata bool
}

// IsStub reports whether name is one of the exercise's stub file names.
func (e *Exercise) IsStub(name string) bool {
	_, found := slices.BinarySearch(e.Stubs, name)
	return found
}

// LoadCourse indexes the course checkout in dir. It fails when the checkout
// has no go.mod, no exercises, or an exercise without stubs or tests, since
// the runner cannot serve such a course.
func LoadCourse(dir string) (*Course, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("course dir %q: %w", dir, err)
	}
	goMod, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		return nil, fmt.Errorf("read course go.mod: %w", err)
	}

	entries, err := os.ReadDir(filepath.Join(abs, "exercises"))
	if err != nil {
		return nil, fmt.Errorf("read course exercises: %w", err)
	}
	c := &Course{dir: abs, goMod: goMod, exercises: make(map[string]*Exercise)}
	for _, entry := range entries {
		if !entry.IsDir() || !exerciseDirRE.MatchString(entry.Name()) {
			continue
		}
		ex, err := loadExercise(filepath.Join(abs, "exercises", entry.Name()))
		if err != nil {
			return nil, err
		}
		c.exercises[ex.Name] = ex
	}
	if len(c.exercises) == 0 {
		return nil, fmt.Errorf("course %s: no exercises found", abs)
	}
	return c, nil
}

func loadExercise(dir string) (*Exercise, error) {
	name := filepath.Base(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("exercise %s: %w", name, err)
	}
	ex := &Exercise{Name: name, Dir: dir}
	for _, entry := range entries {
		file := entry.Name()
		switch {
		case entry.IsDir():
			if file == "testdata" {
				ex.HasTestdata = true
			}
		case !entry.Type().IsRegular() || !strings.HasSuffix(file, ".go"):
			// README.md and anything that is not a plain Go file stays out:
			// only Go files reach the workspace.
		case strings.HasSuffix(file, "_test.go"):
			ex.Tests = append(ex.Tests, file)
		default:
			ex.Stubs = append(ex.Stubs, file)
		}
	}
	if len(ex.Stubs) == 0 || len(ex.Tests) == 0 {
		return nil, fmt.Errorf("exercise %s: needs at least 1 stub and 1 test file", name)
	}
	// os.ReadDir sorts by name already; IsStub relies on it, so make the
	// order explicit instead of implicit.
	slices.Sort(ex.Stubs)
	slices.Sort(ex.Tests)
	return ex, nil
}

// Exercise returns the exercise with the given directory name.
func (c *Course) Exercise(name string) (*Exercise, bool) {
	ex, ok := c.exercises[name]
	return ex, ok
}

// Exercises returns the exercise names in order.
func (c *Course) Exercises() []string {
	names := make([]string, 0, len(c.exercises))
	for name := range c.exercises {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Dir returns the absolute path of the course checkout.
func (c *Course) Dir() string { return c.dir }

// copyCourseFiles copies the exercise's tests and testdata into pkgDir and
// the course go.mod into modDir.
func (c *Course) copyCourseFiles(ex *Exercise, modDir, pkgDir string) error {
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), c.goMod, 0o600); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}
	for _, name := range ex.Tests {
		// The copy is private to the runner user: nothing else on the host
		// needs to read a workspace.
		if err := copyFile(filepath.Join(ex.Dir, name), filepath.Join(pkgDir, name), 0o600); err != nil {
			return err
		}
	}
	if ex.HasTestdata {
		src := filepath.Join(ex.Dir, "testdata")
		if err := os.CopyFS(filepath.Join(pkgDir, "testdata"), os.DirFS(src)); err != nil {
			return fmt.Errorf("copy testdata of %s: %w", ex.Name, err)
		}
	}
	return nil
}
