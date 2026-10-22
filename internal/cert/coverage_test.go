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
		"tier2":    "Tier 2, The Key",
		"tier10":   "Tier 10, The Key Turns",
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
	if cv.Tier == nil || cv.Tier.N != 6 || cv.Title() != "Tier 6, Forensics" {
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
	if d.Title != "Tier 6, Forensics" || d.Tier != 6 || d.TierName != "Forensics" || d.TierLine != "you read what the machine wrote, every quote and every column" ||
		d.Range != "26 to 28" || d.Count != 3 || len(d.Items) != 3 || d.Items[0].Number != "26" || d.Items[2].Title != "Percentiles and histogram quantiles" {
		t.Errorf("document = %+v", d)
	}
	// Its PDF of then says Tier 1, 01 to 03: the new rendering says so too.
	const issuedAs = "Issued as Tier 1 of the first course, exercises 01 to 03. The current course numbers them 26 to 28, in Tier 6, Forensics."
	if got := cv.IssuedAs(); got != issuedAs || d.IssuedAs != issuedAs {
		t.Errorf("IssuedAs = %q, document %q", got, d.IssuedAs)
	}
	// In the first course it is still tier 1, as issued.
	cv = Interpret(first, p)
	if cv.Tier == nil || cv.Tier.N != 1 || cv.Title() != "Tier 1" || !Current(first, "tier1", cv) {
		t.Errorf("in the first course: %v %q", cv.Tier, cv.Title())
	}
	if got := cv.IssuedAs(); got != "" {
		t.Errorf("IssuedAs in the first course = %q", got)
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
	if d.Title != "The complete course" || len(d.Bars) != 25 || len(d.Items) != 0 || d.Bars[0].Tier != 6 || d.Range != "26 to 50" {
		t.Errorf("document = %+v", d)
	}
	const issuedAs = "Issued for the first course: 25 exercises in 5 tiers, numbered 01 to 25. The current course numbers them 26 to 50, in tiers 6 to 10."
	if d.IssuedAs != issuedAs {
		t.Errorf("IssuedAs = %q", d.IssuedAs)
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
	if len(d.Bars) != 50 || len(d.Items) != 0 || d.Range != "01 to 50" || d.IssuedAs != "" {
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
	if !slices.ContainsFunc(d.Items, func(it Item) bool { return it.Number == "32" }) || d.Stats == nil || d.Stats.Runs != 9 || d.Stats.Hints != 2 {
		t.Errorf("items = %+v, stats %+v", d.Items, d.Stats)
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
			[]string{"COURSE CERTIFICATE", "held the Key through 50 exercises", "YEAR 2", "T10", "01 to 50"}},
		{"fifty-course-stats.pdf", fifty, KindCourse, slugsOf(t, fifty, KindCourse), true,
			[]string{"COURSE CERTIFICATE", "T5", "EXPLANATIONS", "WALKTHROUGHS"}},
		{"fifty-old-course.pdf", fifty, KindCourse, slugsOf(t, first, KindCourse), false,
			[]string{"held the Key through 25 exercises", "T6", "26 to 50", "Issued for the first course: 25 exercises in 5 tiers, numbered 01 to 25."}},
		{"fifty-old-course-stats.pdf", fifty, KindCourse, slugsOf(t, first, KindCourse), true,
			[]string{"held the Key through 25 exercises", "T10", "YEAR 2", "ACTIVE"}},
		{"fifty-old-tier1.pdf", fifty, "tier1", slugsOf(t, first, "tier1"), false,
			[]string{"TIER CERTIFICATE", "FORENSICS", "completed Tier 6, Forensics, of the go-practice course", "Parse logfmt log lines", "26 to 28", "Issued as Tier 1 of the first course, exercises 01 to 03."}},
		{"fifty-tier2-stats.pdf", fifty, "tier2", slugsOf(t, fifty, "tier2"), true,
			[]string{"completed Tier 2, The Key, of the go-practice course", "Which nodes went dark?", "06 to 10", "RUNS"}},
		{"fifty-tier10.pdf", fifty, "tier10", slugsOf(t, fifty, "tier10"), false,
			[]string{"completed Tier 10, The Key Turns, of the go-practice", "Capstone: a health-aware load balancer", "45 to 50"}},
		{"first-course.pdf", first, KindCourse, slugsOf(t, first, KindCourse), true,
			[]string{"COURSE CERTIFICATE", "held the Key through 25 exercises", "YEAR 1", "T5"}},
		{"first-tier2.pdf", first, "tier2", slugsOf(t, first, "tier2"), false,
			[]string{"completed Tier 2 of the go-practice course.", "Typed config from the environment", "04 to 09"}},
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
		// The body ends above the foot.
		if l, err := drawSheet(testCanvas(t), d); err != nil || l.contentBottom > l.bodyBottom {
			t.Errorf("%s: the body ends at %.0f, below the foot at %.0f (%v)", tc.file, l.contentBottom, l.bodyBottom, err)
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
		// pdftotext reads a kerned or letter-spaced run as several words,
		// so the words are compared without their spaces.
		squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
		for _, w := range tc.want {
			if !strings.Contains(squash(string(out)), squash(w)) {
				t.Errorf("%s lacks %q:\n%s", tc.file, w, out)
			}
		}
	}
}
