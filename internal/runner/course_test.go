package runner

import (
	"cmp"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// courseDir is the course submodule, relative to this package, or the
// checkout GO_PRACTICE_COURSE names, as for the other packages that test
// the course the site serves.
var courseDir = cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course"))

func loadTestCourse(t *testing.T) *Course {
	t.Helper()
	c, err := LoadCourse(courseDir)
	if err != nil {
		t.Fatalf("LoadCourse: %v", err)
	}
	return c
}

func TestLoadCourseIndexesExercises(t *testing.T) {
	c := loadTestCourse(t)
	if n := len(c.Exercises()); n != 50 {
		t.Errorf("%d exercises, want 50", n)
	}
	tests := []struct {
		name         string
		wantStubs    []string
		wantTests    []string
		wantTestdata bool
	}{
		{name: "01-loglevels", wantStubs: []string{"loglevels.go"}, wantTestdata: true},
		{name: "26-logfmt", wantStubs: []string{"logfmt.go"}, wantTests: []string{"logfmt_test.go", "record_test.go"}},
		{name: "27-accesslog", wantTestdata: true},
		{name: "37-workerpool", wantStubs: []string{"group.go", "workerpool.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex, ok := c.Exercise(tt.name)
			if !ok {
				t.Fatalf("exercise %s not found", tt.name)
			}
			if tt.wantStubs != nil && !reflect.DeepEqual(ex.Stubs, tt.wantStubs) {
				t.Errorf("Stubs = %v, want %v", ex.Stubs, tt.wantStubs)
			}
			if tt.wantTests != nil && !reflect.DeepEqual(ex.Tests, tt.wantTests) {
				t.Errorf("Tests = %v, want %v", ex.Tests, tt.wantTests)
			}
			if ex.HasTestdata != tt.wantTestdata {
				t.Errorf("HasTestdata = %v, want %v", ex.HasTestdata, tt.wantTestdata)
			}
			for _, name := range ex.Stubs {
				if strings.HasSuffix(name, "_test.go") || !ex.IsStub(name) {
					t.Errorf("stub %q misclassified", name)
				}
			}
		})
	}
	if _, ok := c.Exercise("../exercises/26-logfmt"); ok {
		t.Error("a path-like exercise name was found")
	}
}

func TestLoadCourseRejectsBrokenCheckouts(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{name: "no go.mod", files: map[string]string{"exercises/01-a/a.go": "", "exercises/01-a/a_test.go": ""}},
		{name: "no exercises", files: map[string]string{"go.mod": "module m\n", "exercises/README.md": ""}},
		{name: "exercise without tests", files: map[string]string{"go.mod": "module m\n", "exercises/01-a/a.go": ""}},
		{name: "exercise without stubs", files: map[string]string{"go.mod": "module m\n", "exercises/01-a/a_test.go": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeTestFile(t, filepath.Join(dir, name), content)
			}
			if _, err := LoadCourse(dir); err == nil {
				t.Error("LoadCourse succeeded, want an error")
			}
		})
	}
}

func TestNewWorkspaceLayout(t *testing.T) {
	c := loadTestCourse(t)
	ex, _ := c.Exercise("27-accesslog")
	files := map[string]string{}
	for _, name := range ex.Stubs {
		files[name] = "package accesslog\n"
	}
	workDir := t.TempDir()
	ws, err := newWorkspace(workDir, c, ex, files)
	if err != nil {
		t.Fatalf("newWorkspace: %v", err)
	}
	want := []string{
		"mod/go.mod",
		"mod/exercises/27-accesslog/testdata/incident.log",
	}
	for _, name := range append(ex.Stubs, ex.Tests...) {
		want = append(want, "mod/exercises/27-accesslog/"+name)
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(ws.root, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if err := ws.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if entries, _ := os.ReadDir(workDir); len(entries) != 0 {
		t.Errorf("work dir not empty after Close: %v", entries)
	}
}

func TestRemoveStaleWorkspacesKeepsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "run-123/mod/x.go"), "x")
	writeTestFile(t, filepath.Join(dir, "keep.txt"), "x")
	// A test can leave a read-only directory behind.
	if err := os.Chmod(filepath.Join(dir, "run-123/mod"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := RemoveStaleWorkspaces(dir); err != nil {
		t.Fatalf("RemoveStaleWorkspaces: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "run-123")); !os.IsNotExist(err) {
		t.Errorf("stale workspace still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Errorf("unrelated file removed: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
