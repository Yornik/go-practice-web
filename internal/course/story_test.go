package course_test

import (
	"slices"
	"testing"

	"github.com/yornik/go-practice-web/internal/course/coursetest"
)

// TestTierTags checks the tags of the 10 tiers against VESPER.md 16.1:
// the stamp, the lines the ring's tag sets, and the story's calendar.
func TestTierTags(t *testing.T) {
	c := coursetest.Load(t, coursetest.Fifty())
	want := []struct {
		label, stamp string
		lines        []string
		year         int
		when, month  string
	}{
		{"Tier 1, Badge In", "T1 · BADGE IN", []string{"BADGE IN"}, 1, "September", "September"},
		{"Tier 2, The Key", "T2 · THE KEY", []string{"THE KEY"}, 1, "October", "October"},
		{"Tier 3, Hand Tools", "T3 · HAND TOOLS", []string{"HAND", "TOOLS"}, 1, "November", "November"},
		{"Tier 4, Surge", "T4 · SURGE", []string{"SURGE"}, 1, "Launch Week", "Launch Week"},
		{"Tier 5, Dead Air", "T5 · DEAD AIR", []string{"DEAD AIR"}, 1, "December to 1 January", "December"},
		{"Tier 6, Forensics", "T6 · FORENSICS", []string{"FORENSICS"}, 2, "January and February", "January"},
		{"Tier 7, Containment", "T7 · CONTAINMENT", []string{"CONTAINMENT"}, 2, "March to May", "Spring"},
		{"Tier 8, Blast Radius", "T8 · BLAST RADIUS", []string{"BLAST", "RADIUS"}, 2, "June and July", "Summer"},
		{"Tier 9, Clean Signal", "T9 · CLEAN SIGNAL", []string{"CLEAN", "SIGNAL"}, 2, "August to mid-October", "Autumn"},
		{"Tier 10, The Key Turns", "T10 · THE KEY TURNS", []string{"THE KEY", "TURNS"}, 2, "To the night the clocks go back", "The last week of October"},
	}
	if len(c.Tiers) != len(want) {
		t.Fatalf("%d tiers", len(c.Tiers))
	}
	for i, w := range want {
		tr := c.Tiers[i]
		if tr.Label() != w.label || tr.Stamp() != w.stamp || !slices.Equal(tr.StampLines(), w.lines) ||
			tr.Year() != w.year || tr.When() != w.when || tr.Month() != w.month {
			t.Errorf("tier %d = %q %q %q %d %q %q", tr.N, tr.Label(), tr.Stamp(), tr.StampLines(), tr.Year(), tr.When(), tr.Month())
		}
		if tr.Line() == "" || tr.LineSentence() == "" {
			t.Errorf("tier %d has no certificate line", tr.N)
		}
	}
	t1, _ := c.Tier(1)
	if got := t1.LineSentence(); got != "You read the raw lines the dashboards miss." {
		t.Errorf("tier 1 line = %q", got)
	}
	t10, _ := c.Tier(10)
	if got := t10.Line(); got != "your tools held the city's night" {
		t.Errorf("tier 10 line = %q", got)
	}
}

// TestTierTagsWithoutNames: the first course has no tier names, so its
// tags carry only the number and the story gives them nothing.
func TestTierTagsWithoutNames(t *testing.T) {
	c := coursetest.Load(t, coursetest.First())
	t2, _ := c.Tier(2)
	if t2.Stamp() != "T2" || t2.StampLines() != nil || t2.Month() != "" || t2.When() != "" || t2.Line() != "" || t2.LineSentence() != "" {
		t.Errorf("tier 2 = %q %q %q %q %q", t2.Stamp(), t2.StampLines(), t2.Month(), t2.When(), t2.Line())
	}
}
