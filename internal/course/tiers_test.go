package course_test

import (
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
	if t2.Name != "Carrying the Owl" || t2.Label() != "Tier 2 · Carrying the Owl" || t2.Title() != "Tier 2" {
		t.Errorf("tier 2 = %q %q", t2.Name, t2.Label())
	}
	if !strings.HasPrefix(t2.Theme, "Types with methods") {
		t.Errorf("tier 2 theme = %q", t2.Theme)
	}
	t10, ok := c.Tier(10)
	if !ok || t10.Label() != "Tier 10 · Passing the Owl" || t10.Range() != "45–50" || len(t10.Exercises) != 6 {
		t.Fatalf("tier 10 = %+v", t10)
	}
	if t10.Exercises[0].Tier != 10 || t10.Exercises[0].Slug != "45-singleflight" {
		t.Errorf("tier 10 starts with %s (tier %d)", t10.Exercises[0].Slug, t10.Exercises[0].Tier)
	}
	e, renamed, ok := c.Resolve("07-retry")
	if !ok || !renamed || e.Slug != "32-retry" || e.Number != 32 || e.Tier != 7 {
		t.Errorf("Resolve(07-retry) = %+v %v %v", e, renamed, ok)
	}
	if e, renamed, ok := c.Resolve("07-inventory"); !ok || renamed || e.Number != 7 {
		t.Errorf("Resolve(07-inventory) = %v %v %v", e, renamed, ok)
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
// course: either every target exists (the course of 50), or the table is
// inactive because every old slug is still current (the first course).
func TestRenamesAgainstTheCourse(t *testing.T) {
	c, err := course.Load(filepath.Join("..", "..", "course"))
	if err != nil {
		t.Fatal(err)
	}
	renames := course.Renames()
	if len(renames) != 25 {
		t.Fatalf("the table has %d renames, want 25", len(renames))
	}
	var old, moved int
	for from, to := range renames {
		_, fromOK := c.Exercise(from)
		_, toOK := c.Exercise(to)
		switch {
		case fromOK && !toOK:
			old++
		case toOK && !fromOK:
			moved++
		default:
			t.Errorf("%s -> %s: the course has %v and %v", from, to, fromOK, toOK)
		}
		if from[2:] != to[2:] {
			t.Errorf("%s -> %s changes the name", from, to)
		}
	}
	if old != 0 && moved != 0 {
		t.Errorf("the course has %d old and %d new slugs; want all of 1 kind", old, moved)
	}
	t.Logf("old slugs current: %d, renamed: %d", old, moved)
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
	broken := strings.Replace(string(b), "| 2 | 06–10 | Carrying the Owl |", "| 2 | 06–10 |", 1)
	if err := os.WriteFile(p, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := course.Load(dir); err == nil {
		t.Error("Load accepted a tier row with 3 cells under a 4-column header")
	}
}
