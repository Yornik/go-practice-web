package course_test

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
)

// TestFirstCourseShape loads the fixture of the first course: 5 tiers
// without names, from the 3-column tier table.
func TestFirstCourseShape(t *testing.T) {
	c := coursetest.Load(t, coursetest.First())
	if len(c.Exercises) != 25 || len(c.Tiers) != 5 {
		t.Fatalf("got %d exercises in %d tiers", len(c.Exercises), len(c.Tiers))
	}
	t2, _ := c.Tier(2)
	if t2.Name != "" || t2.Label() != "Tier 2" || t2.Title() != "Tier 2" || t2.Range() != "04–09" {
		t.Errorf("tier 2 = %q %q %q", t2.Name, t2.Label(), t2.Range())
	}
	// The old slugs are current: nothing resolves through the table.
	e, renamed, ok := c.Resolve("07-retry")
	if !ok || renamed || e.Slug != "07-retry" {
		t.Errorf("Resolve(07-retry) = %v %v %v", e, renamed, ok)
	}
	if _, _, ok := c.Resolve("32-retry"); ok {
		t.Error("Resolve(32-retry) found an exercise in the first course")
	}
}

// TestFiftyShape loads the fixture of the course of 50: 10 named tiers,
// tier numbers above 5 on line 3, and the old slugs renamed.
func TestFiftyShape(t *testing.T) {
	c := coursetest.Load(t, coursetest.Fifty())
	if len(c.Exercises) != 50 || len(c.Tiers) != 10 {
		t.Fatalf("got %d exercises in %d tiers", len(c.Exercises), len(c.Tiers))
	}
	t2, _ := c.Tier(2)
	if t2.Name != "The Key" || t2.Label() != "Tier 2 · The Key" || t2.Title() != "Tier 2" {
		t.Errorf("tier 2 = %q %q", t2.Name, t2.Label())
	}
	if !strings.HasPrefix(t2.Theme, "Types with methods") {
		t.Errorf("tier 2 theme = %q", t2.Theme)
	}
	t10, ok := c.Tier(10)
	if !ok || t10.Label() != "Tier 10 · The Key Turns" || t10.Range() != "45–50" || len(t10.Exercises) != 6 {
		t.Fatalf("tier 10 = %+v", t10)
	}
	if t10.Exercises[0].Tier != 10 || t10.Exercises[0].Slug != "45-singleflight" {
		t.Errorf("tier 10 starts with %s (tier %d)", t10.Exercises[0].Slug, t10.Exercises[0].Tier)
	}
	e, renamed, ok := c.Resolve("07-retry")
	if !ok || !renamed || e.Slug != "32-retry" || e.Number != 32 || e.Tier != 7 {
		t.Errorf("Resolve(07-retry) = %+v %v %v", e, renamed, ok)
	}
	if e, renamed, ok := c.Resolve("07-rollcall"); !ok || renamed || e.Number != 7 {
		t.Errorf("Resolve(07-rollcall) = %v %v %v", e, renamed, ok)
	}
	if got := c.CurrentSlug("01-logfmt"); got != "26-logfmt" {
		t.Errorf("CurrentSlug(01-logfmt) = %q", got)
	}
	if got := c.CurrentSlug("99-nothing"); got != "" {
		t.Errorf("CurrentSlug(99-nothing) = %q", got)
	}
	if _, _, ok := c.Resolve("../07-retry"); ok {
		t.Error("Resolve found a path")
	}
}

// TestRenamesAgainstTheCourse checks the rename table against the real
// course: every old slug is gone and every new one is an exercise with the
// same name.
func TestRenamesAgainstTheCourse(t *testing.T) {
	c, err := course.Load(cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course")))
	if err != nil {
		t.Fatal(err)
	}
	renames := course.Renames()
	if len(renames) != 25 {
		t.Fatalf("the table has %d renames, want 25", len(renames))
	}
	for from, to := range renames {
		if _, ok := c.Exercise(from); ok {
			t.Errorf("the course still has the old slug %s", from)
		}
		e, ok := c.Exercise(to)
		if !ok {
			t.Errorf("%s -> %s: the course has no %s", from, to, to)
			continue
		}
		if from[2:] != to[2:] {
			t.Errorf("%s -> %s changes the name", from, to)
		}
		if e.Tier < 6 {
			t.Errorf("%s is in tier %d, want 6 to 10", to, e.Tier)
		}
		if got, renamed, ok := c.Resolve(from); !ok || !renamed || got != e {
			t.Errorf("Resolve(%s) = %v %v %v", from, got, renamed, ok)
		}
	}
}

// TestTierTableErrors refuses a tier table with a row that has the wrong
// number of cells.
func TestTierTableErrors(t *testing.T) {
	dir := t.TempDir()
	if err := coursetest.Write(dir, coursetest.Fifty()); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "README.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(b), "| 2 | 06–10 | The Key |", "| 2 | 06–10 |", 1)
	if err := os.WriteFile(p, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := course.Load(dir); err == nil {
		t.Error("Load accepted a tier row with 3 cells under a 4-column header")
	}
}
