package cert

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/course"
)

// slugsOf returns the slugs of kind k in course c.
func slugsOf(t *testing.T, c *course.Course, k Kind) []string {
	t.Helper()
	exs, err := Exercises(c, k)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(exs))
	for i, e := range exs {
		out[i] = e.Slug
	}
	return out
}

func numbers(exs []*course.Exercise) []int {
	out := make([]int, len(exs))
	for i, e := range exs {
		out[i] = e.Number
	}
	return out
}

func TestKindsOfFifty(t *testing.T) {
	kinds := Kinds(fifty)
	if len(kinds) != 11 || kinds[9] != "tier10" || kinds[10] != KindCourse {
		t.Fatalf("Kinds = %v", kinds)
	}
	for k, want := range map[Kind]string{
		"tier2":    "Tier 2 · Carrying the Owl",
		"tier10":   "Tier 10 · Passing the Owl",
		KindCourse: "The complete course",
	} {
		if got, err := Title(fifty, k); err != nil || got != want {
			t.Errorf("Title(%s) = %q, %v; want %q", k, got, err, want)
		}
	}
	if _, err := ParseKind(fifty, "tier11"); err == nil {
		t.Error("ParseKind(tier11) succeeded")
	}
	if n := len(slugsOf(t, fifty, KindCourse)); n != 50 {
		t.Errorf("the course kind covers %d exercises", n)
	}
}

// oldPayload is a payload the first course signed.
func oldPayload(t *testing.T, k Kind) Payload {
	t.Helper()
	return Payload{ID: "GP-7Q2M-K9XD-3HFA", Kind: k, Name: "Ada Lovelace", IssuedAt: "2026-09-30T08:00:00Z", Exercises: slugsOf(t, first, k)}
}

func TestInterpretOldTierInFifty(t *testing.T) {
	p := oldPayload(t, "tier1")
	if !slices.Equal(p.Exercises, []string{"01-logfmt", "02-accesslog", "03-latency"}) {
		t.Fatalf("the first course's tier 1 is %v", p.Exercises)
	}
	cv := Interpret(fifty, p)
	if cv.Tier == nil || cv.Tier.N != 6 || cv.Title() != "Tier 6 · The morning after" {
		t.Fatalf("old tier1 reads as %v %q", cv.Tier, cv.Title())
	}
	if got := numbers(cv.Exercises); !slices.Equal(got, []int{26, 27, 28}) || cv.Range() != "26 to 28" || len(cv.Unknown) != 0 {
		t.Errorf("exercises %v, range %q, unknown %v", got, cv.Range(), cv.Unknown)
	}
	// The migration stores it as tier6, which it covers exactly.
	if !Current(fifty, "tier6", cv) {
		t.Error("an old tier1 is not current as tier6")
	}
	if Current(fifty, "tier1", cv) {
		t.Error("an old tier1 is current as the new tier1")
	}

	d, err := NewDocument(fifty, p, "https://go-learn.party", "FP")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Tier 6 · The morning after" || d.Tier != 6 || d.Subtitle != "Warm-up: Go idioms on real data" ||
		d.Range != "26 to 28" || d.Count != 3 || len(d.Items) != 3 || d.Items[0].Number != "26" || d.Items[2].Title != "Percentiles and histogram quantiles" {
		t.Errorf("document = %+v", d)
	}
	// In the first course it is still tier 1.
	cv = Interpret(first, p)
	if cv.Tier == nil || cv.Tier.N != 1 || cv.Title() != "Tier 1" || !Current(first, "tier1", cv) {
		t.Errorf("in the first course: %v %q", cv.Tier, cv.Title())
	}
}

func TestInterpretOldCourseInFifty(t *testing.T) {
	p := oldPayload(t, KindCourse)
	cv := Interpret(fifty, p)
	if cv.Title() != "The complete course" || cv.Summary() != "25 exercises in 5 tiers" || cv.Range() != "26 to 50" {
		t.Errorf("old course reads as %q %q %q", cv.Title(), cv.Summary(), cv.Range())
	}
	if cv.Tier != nil || len(cv.Tiers) != 5 || cv.Tiers[0].N != 6 || cv.Tiers[4].N != 10 {
		t.Errorf("tiers = %v", cv.Tiers)
	}
	if Current(fifty, KindCourse, cv) {
		t.Error("the old course certificate is current in the course of 50")
	}
	if !Current(first, KindCourse, Interpret(first, p)) {
		t.Error("the course certificate is not current in its own course")
	}

	d, err := NewDocument(fifty, p, "https://go-learn.party", "FP")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "The complete course" || d.Subtitle != "25 exercises in 5 tiers" || len(d.Bars) != 25 || len(d.Items) != 5 ||
		d.Items[0].Number != "Tier 6" || d.Items[0].Title != "The morning after" || d.Range != "26 to 50" {
		t.Errorf("document = %+v", d)
	}

	// A new course certificate covers all 50 in 10 tiers.
	np := Payload{ID: p.ID, Kind: KindCourse, Name: p.Name, IssuedAt: p.IssuedAt, Exercises: slugsOf(t, fifty, KindCourse)}
	cv = Interpret(fifty, np)
	if !Current(fifty, KindCourse, cv) || cv.Summary() != "50 exercises in 10 tiers" {
		t.Errorf("new course certificate: current %v, %q", Current(fifty, KindCourse, cv), cv.Summary())
	}
	d, err = NewDocument(fifty, np, "https://go-learn.party", "FP")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Bars) != 50 || len(d.Items) != 10 || d.Items[1].Title != "Carrying the Owl" || d.Range != "01 to 50" {
		t.Errorf("document = %+v", d)
	}
	if d.Bars[49].Tier != 10 || d.Bars[0].Tier != 1 {
		t.Errorf("bars run from tier %d to %d", d.Bars[0].Tier, d.Bars[49].Tier)
	}
}

func TestInterpretStatsAndUnknown(t *testing.T) {
	p := oldPayload(t, "tier2")
	p.Stats = BuildStats(p.Exercises, map[string]Activity{"07-retry": {ActiveSeconds: 7200, Runs: 9, Levels: []int{1, 2}}})
	p.Exercises = append(p.Exercises, "99-gone")
	cv := Interpret(fifty, p)
	if s, ok := cv.Stats["32-retry"]; !ok || s.Runs != 9 || s.Slug != "07-retry" {
		t.Errorf("stats of 32-retry = %+v, %v", s, ok)
	}
	if !slices.Equal(cv.Unknown, []string{"99-gone"}) || Current(fifty, "tier7", cv) {
		t.Errorf("unknown %v, current %v", cv.Unknown, Current(fifty, "tier7", cv))
	}
	d, err := NewDocument(fifty, p, "https://go-learn.party", "FP")
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(d.Items, func(it Item) bool { return it.Number == "32" }); i < 0 || d.Items[i].Detail != "2 h, 9 test runs, 2 hints" {
		t.Errorf("items = %+v", d.Items)
	}

	// A tier payload whose exercises span 2 tiers keeps its kind's number.
	span := Payload{ID: p.ID, Kind: "tier3", Name: "x", IssuedAt: p.IssuedAt, Exercises: []string{"03-latency", "04-grep"}}
	if got := Interpret(first, span).Title(); got != "Tier 3" {
		t.Errorf("a payload across tiers reads as %q", got)
	}
	d, err = NewDocument(first, span, "https://go-learn.party", "FP")
	if err != nil || d.Tier != 0 || d.Range != "03 to 04" {
		t.Errorf("document across tiers = %+v, %v", d, err)
	}
}

// TestRenderLayouts renders the certificates whose layout changed with the
// course of 50 and checks the text in them with pdftotext when it is
// installed. CERT_SAMPLES_DIR keeps the files (and a PNG of each when
// pdftoppm is installed) for a look by eye.
func TestRenderLayouts(t *testing.T) {
	s := testSigner(t)
	dir := os.Getenv("CERT_SAMPLES_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	issued := time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)
	activity := func(slugs []string) map[string]Activity {
		a := map[string]Activity{}
		for i, slug := range slugs {
			a[slug] = Activity{ActiveSeconds: int64(3600*(i%4) + 1200*i + 900), Runs: 4 + 3*i, Levels: []int{1, 2, 3, 4, 5, 6}[:i%5]}
		}
		return a
	}
	cases := []struct {
		file  string
		c     *course.Course
		kind  Kind
		slugs []string
		stats bool
		want  []string
	}{
		{"fifty-course.pdf", fifty, KindCourse, slugsOf(t, fifty, KindCourse), false,
			[]string{"The complete course", "50 exercises in 10 tiers", "Tier 10", "Passing the Owl", "01 to 50"}},
		{"fifty-course-stats.pdf", fifty, KindCourse, slugsOf(t, fifty, KindCourse), true,
			[]string{"The complete course", "50 exercises in 10 tiers", "Tier 5", "The long nights", "Explanations"}},
		{"fifty-old-course.pdf", fifty, KindCourse, slugsOf(t, first, KindCourse), false,
			[]string{"25 exercises in 5 tiers", "Tier 6", "The morning after", "26 to 50"}},
		{"fifty-old-course-stats.pdf", fifty, KindCourse, slugsOf(t, first, KindCourse), true,
			[]string{"25 exercises in 5 tiers", "Tier 10", "Passing the Owl"}},
		{"fifty-old-tier1.pdf", fifty, "tier1", slugsOf(t, first, "tier1"), false,
			[]string{"Tier 6 · The morning after", "tier 6", "Parse logfmt log lines", "26 to 28"}},
		{"fifty-tier2-stats.pdf", fifty, "tier2", slugsOf(t, fifty, "tier2"), true,
			[]string{"Tier 2 · Carrying the Owl", "Types with methods", "06 to 10"}},
		{"fifty-tier10.pdf", fifty, "tier10", slugsOf(t, fifty, "tier10"), false,
			[]string{"Tier 10 · Passing the Owl", "Capstone: a health-aware load balancer", "45 to 50"}},
		{"first-course.pdf", first, KindCourse, slugsOf(t, first, KindCourse), true,
			[]string{"The complete course", "25 exercises in 5 tiers", "Warm-up: Go idioms on real data"}},
		{"first-tier2.pdf", first, "tier2", slugsOf(t, first, "tier2"), false,
			[]string{"Tier 2", "Streams, state, time, types, errors and locks", "04 to 09"}},
	}
	text, haveText := exec.LookPath("pdftotext")
	png, havePNG := exec.LookPath("pdftoppm")
	for _, tc := range cases {
		var st *Stats
		if tc.stats {
			st = BuildStats(tc.slugs, activity(tc.slugs))
		}
		crt, err := s.Issue(Request{Kind: tc.kind, Name: "Grace Brewster Murray Hopper", Exercises: tc.slugs, Stats: st, IssuedAt: issued})
		if err != nil {
			t.Fatal(err)
		}
		d, err := NewDocument(tc.c, crt.Payload, "https://go-learn.party", s.Fingerprint())
		if err != nil {
			t.Fatal(err)
		}
		// The list ends above the drawing area's bottom edge, with room
		// for the descenders.
		if top := areaT + max(-headSlack, ((areaB-areaT)-(headH+listHeight(d))-30)/2); top+headH+listHeight(d) > areaB-4 {
			t.Errorf("%s: the list ends at %.0f, below the area at %.0f", tc.file, top+headH+listHeight(d), areaB)
		}
		var buf bytes.Buffer
		if err := Render(&buf, d); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, tc.file)
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if havePNG == nil {
			_ = exec.Command(png, "-png", "-r", "60", "-singlefile", path, strings.TrimSuffix(path, ".pdf")).Run()
		}
		if haveText != nil {
			continue
		}
		out, err := exec.Command(text, "-layout", path, "-").Output()
		if err != nil {
			t.Fatalf("pdftotext %s: %v", tc.file, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(string(out), w) {
				t.Errorf("%s lacks %q:\n%s", tc.file, w, out)
			}
		}
	}
}
