package web

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
	"github.com/yornik/go-practice-web/internal/store"
)

// The learners of the frames: Ada in tier 2 (dashboard.html) and on day 1
// (dashboard-day1.html), Łukasz on the certificates page. Every time is
// fixed, so the story is the same on every run.

// storyCourse loads the fixture of the course of 50: the tiers, names and
// hours of the real course.
func storyCourse(t *testing.T) *course.Course {
	t.Helper()
	return coursetest.Load(t, coursetest.Fifty())
}

var (
	now  = time.Date(2026, 10, 22, 21, 30, 0, 0, time.UTC)
	ada  = store.User{ID: 1, DisplayName: "Ada Lovelace", CreatedAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)}
	day  = func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }
	slug = func(c *course.Course, n int) string { e, _ := c.ExerciseByNumber(n); return e.Slug }
)

// passed is a passed exercise's row, passed on at.
func passed(c *course.Course, n int, at time.Time) store.Progress {
	return store.Progress{Exercise: slug(c, n), Status: store.StatusPassed, StartedAt: at.Add(-time.Hour), PassedAt: at,
		LastRunAt: at, ActiveSeconds: 3600, Runs: 5}
}

// started is a started exercise's row, last run at at.
func started(c *course.Course, n int, at time.Time) store.Progress {
	return store.Progress{Exercise: slug(c, n), Status: store.StatusStarted, StartedAt: at.Add(-time.Hour), LastRunAt: at}
}

// certs builds what certificateStatus answers: the tiers held, current,
// with their issue dates, and optionally an earlier course certificate.
func certs(c *course.Course, name string, tiers map[int]time.Time, course *time.Time, earlier bool) []certStatus {
	var out []certStatus
	for _, k := range cert.Kinds(c) {
		st := certStatus{Kind: string(k)}
		j := func(at time.Time) *certJSON {
			id := fmt.Sprintf("GP-%s-0000-0000", strings.ToUpper(string(k))[:4])
			return &certJSON{ID: id, Name: name, IssuedAt: at, URL: verifyPath(id), PDF: verifyPath(id) + ".pdf"}
		}
		if n, ok := k.Tier(); ok {
			if at, ok := tiers[n]; ok {
				st.Certificate = j(at)
			}
		} else {
			if course != nil {
				st.Certificate = j(*course)
			}
			if earlier {
				st.Earlier = j(day(6, 27))
			}
		}
		out = append(out, st)
	}
	return out
}

// adaInTier2 is dashboard.html: 01 to 06 pass, T1 stamped on 22 Sep, on
// call for 07 with 34 minutes and hint 1.
func adaInTier2(c *course.Course) storyInput {
	in := storyInput{Course: c, User: ada, Now: now, Certs: certs(c, "Ada Lovelace", map[int]time.Time{1: day(9, 22)}, nil, false)}
	for n := 1; n <= 6; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(9, 10+n)))
	}
	p := started(c, 7, day(10, 20))
	p.ActiveSeconds, p.Runs = 34*60, 4
	in.Progress = append(in.Progress, p)
	in.Reveals = []store.Reveal{{Exercise: slug(c, 7), Level: 1, RevealedAt: day(10, 19)}}
	return in
}

func TestStoryDay1(t *testing.T) {
	c := storyCourse(t)
	s := buildStory(storyInput{Course: c, User: store.User{DisplayName: "Ada Lovelace", CreatedAt: now.Add(-time.Hour)},
		Now: now, Certs: certs(c, "", nil, nil, false)})
	if !s.Day1 || s.KeyOnRing || s.Stamped != 0 || s.OnCall.Number != 1 || s.Open == nil || s.Open.N != 1 || !s.Open.IsOpen() {
		t.Fatalf("day 1: Day1 %v, Key %v, stamped %d, on call %v, open %v", s.Day1, s.KeyOnRing, s.Stamped, s.OnCall, s.Open)
	}
	if want := "Your ring: empty, on its lanyard. The Key hangs on its hook inside the dial, with its B0 tag. A dashed outline for T1, Badge In, with 5 exercises to go. The tier dial: no tier stamped yet."; s.Ring.Title != want {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	if s.Ring.Count != "0 of 10 tags" || s.Ring.Line != "The Key is on its hook. It clips onto your ring in tier 2." ||
		len(s.Ring.Items) != 1 || s.Ring.Items[0].Text != "5 to go" || s.Ring.Items[0].N != 1 {
		t.Errorf("ring card = %q %q %+v", s.Ring.Count, s.Ring.Line, s.Ring.Items)
	}
	if !strings.Contains(string(s.Ring.SVG), `data-ring-part="hook"`) || !strings.Contains(string(s.Ring.SVG), "translate(116 160)") {
		t.Error("the day-1 ring has no hook at its frame place")
	}
	wantSlab := slab{State: "new", MarkLabel: "Day 1, floor 61", Lead: "Your next exercise", Exercise: s.OnCall,
		Button: "Start exercise 01", Href: "/learn/01-loglevels",
		Facts: []fact{{"Tier", "1, Badge In"}, {"Estimate", "1 h"}, {"Active", "–"}, {"Revealed", "Nothing"}}}
	if fmt.Sprint(s.Slab) != fmt.Sprint(wantSlab) {
		t.Errorf("slab = %+v", s.Slab)
	}
	l := s.Ledger
	if l.Said != "0 of 50 tools are in the Ledger." || l.Active != "None yet" || l.Runs != 0 || l.Hints != 0 || l.Since != "Today" ||
		!strings.HasSuffix(string(l.Intro), "Your ring is empty, and the Key is on its hook.") || l.Rest != "Tiers 2 to 10" {
		t.Errorf("ledger = %+v", l)
	}
	r := s.Readout
	if r.Ledger != "Ada Lovelace, junior SRE" || r.MonthLine != "September, tier 1" || r.Month != "September" || r.Tier != 1 ||
		r.KeyOnRing || r.Rotation != "The Key is on its hook" {
		t.Errorf("readout = %+v", r)
	}
	if b := s.Badge; b.TagText() != "Day 1" || b.Initials != "AL" || b.Label != "Ada Lovelace (AL), day 1, no tags yet. Account menu." {
		t.Errorf("badge = %+v", b)
	}
	if s.Calm.Score != "99.2" || s.Calm.Moved || s.Calm.Say == "" || s.Calm.Caption != "It said 99.2 on your first day too." {
		t.Errorf("calm = %+v", s.Calm)
	}
	sky := s.Skyline
	if sky.Caption != "0 in the Ledger, 01 is next" || sky.CurWord != "Next" || sky.OffWord != "Not started" ||
		sky.Label != "Your skyline: 50 towers, 1 per exercise, as tall as its estimate. Tower 01 is amber: it is next. The other 49 are outlines: not started." {
		t.Errorf("skyline = %q %q %q %q", sky.Caption, sky.CurWord, sky.OffWord, sky.Label)
	}
}

// TestSkylineLegend: the legend names the outlined towers as the label
// does, and leaves them out when there is none.
func TestSkylineLegend(t *testing.T) {
	c := storyCourse(t)
	in := adaInTier2(c)
	if s := buildStory(in); s.Skyline.OffWord != "Not started" {
		t.Errorf("Ada: %q", s.Skyline.OffWord)
	}
	// 08 started earlier than 07, so 07 stays on call and 08 is an outline.
	in.Progress = append(in.Progress, started(c, 8, day(9, 1)))
	if s := buildStory(in); s.OnCall.Number != 7 || s.Skyline.OffWord != "Not in the Ledger yet" ||
		!strings.HasSuffix(s.Skyline.Label, "are outlines: not in the Ledger yet.") {
		t.Errorf("08 started: on call %v, %q, %q", s.OnCall, s.Skyline.OffWord, s.Skyline.Label)
	}
	in = storyInput{Course: c, User: ada, Now: now}
	for n := 1; n <= 50; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(10, 1)))
	}
	if s := buildStory(in); s.Skyline.OffWord != "" {
		t.Errorf("all pass: %q", s.Skyline.OffWord)
	}
}

func TestStoryAdaInTier2(t *testing.T) {
	c := storyCourse(t)
	s := buildStory(adaInTier2(c))
	if s.Day1 || !s.KeyOnRing || s.OnCall.Number != 7 || s.Open.N != 2 || s.Open.Passed != 1 || s.Open.ToGo() != 4 ||
		s.Stamped != 1 || s.Latest.N != 1 || s.Highest.N != 1 || s.Passed != 6 || s.Started != 7 {
		t.Fatalf("Ada: on call %v, open %v, stamped %d, passed %d", s.OnCall, s.Open, s.Stamped, s.Passed)
	}
	if want := "Your ring: the stamped tag T1, Badge In, hangs on it. The Key hangs beside it, with its B0 tag. A dashed outline for T2, The Key, with 4 exercises to go. The tier dial: 1 of 10 stamped."; s.Ring.Title != want {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	items := s.Ring.Items
	if len(items) != 2 || items[0].Text != "stamped 22 Sep" || items[0].Stamp() != "T1 · BADGE IN" || items[0].TagClass() != "gpw-tag-on" ||
		items[1].Text != "4 to go" || items[1].TagClass() != "gpw-tag-cur" {
		t.Errorf("tag list = %+v", items)
	}
	svg := string(s.Ring.SVG)
	for _, want := range []string{`<title id="ring-title">Your ring: the stamped tag T1`, `data-ring-part="key"`, `data-ring-part="tags"`, "4 TO GO"} {
		if !strings.Contains(svg, want) {
			t.Errorf("the ring lacks %s", want)
		}
	}
	if s.Slab.State != "key" || !s.Slab.Key || s.Slab.MarkLabel != "Breaker 0, Hall 3" || s.Slab.Lead != "You are on call for" ||
		s.Slab.Button != "Continue exercise 07" ||
		fmt.Sprint(s.Slab.Facts) != fmt.Sprint([]fact{{"Tier", "2, The Key"}, {"Estimate", "1 h"}, {"Active", "34 min"}, {"Revealed", "Hint 1"}}) {
		t.Errorf("slab = %+v", s.Slab)
	}
	if s.Ledger.Said != "6 of 50 tools are in the Ledger." || s.Ledger.Since != "2 Sep 2026" || s.Ledger.Rest != "Tiers 3 to 10" ||
		s.Ledger.Runs != 34 || s.Ledger.Hints != 1 || s.Ledger.Active != "6 h 34 min" {
		t.Errorf("ledger = %+v", s.Ledger)
	}
	if s.Readout.MonthLine != "October, tier 2" || s.Readout.Rotation != "On call, with Wren" || s.Readout.Ledger != "Ada Lovelace, junior SRE" {
		t.Errorf("readout = %+v", s.Readout)
	}
	if b := s.Badge; b.TagText() != "T1" || b.Label != "Ada Lovelace (AL), highest tag T1, Badge In. Account menu." || b.Menu != "Highest tag T1, Badge In" {
		t.Errorf("badge = %+v", b)
	}
	if want := "Your skyline: 50 towers, 1 per exercise, as tall as its estimate. Towers 01 to 06 are solid: in the Ledger. Tower 07 is amber: you are on call for it. The other 43 are outlines: not started."; s.Skyline.Label != want {
		t.Errorf("skyline label = %q", s.Skyline.Label)
	}
	if s.Skyline.Caption != "6 in the Ledger, 07 on call" || s.Skyline.CurWord != "On call" {
		t.Errorf("skyline caption = %q %q", s.Skyline.Caption, s.Skyline.CurWord)
	}
	t1, t2 := s.Tiers[0], s.Tiers[1]
	if t1.StampedText() != "Tag stamped 22 Sep" || t1.InLedger() != "5 of 5 in the Ledger" || t1.ToGoForTag() != "" ||
		t2.InLedger() != "1 of 5 in the Ledger" || t2.ToGoForTag() != "4 to go for the tag" ||
		t2.Topics() != "October. Types with methods, errors you can inspect, enums, time values, text as bytes." {
		t.Errorf("tier heads: %q %q %q %q", t1.StampedText(), t1.InLedger(), t2.ToGoForTag(), t2.Topics())
	}
	if t4 := s.Tiers[3]; t4.Topics() != "Launch Week. Interfaces the standard library calls, generics, <code>reflect</code>, goroutines and a mutex." {
		t.Errorf("tier 4 topics = %q", t4.Topics())
	}
}

// TestOnCall checks the rule of VESPER.md 16.2: the exercise worked in
// last that does not pass, whatever left the last trace.
func TestOnCall(t *testing.T) {
	c := storyCourse(t)
	base := func() storyInput {
		return storyInput{Course: c, User: ada, Now: now, Certs: certs(c, "", nil, nil, false),
			Progress: []store.Progress{passed(c, 1, day(9, 3)), started(c, 3, day(9, 5)), started(c, 2, day(9, 4))}}
	}
	on := func(in storyInput) int { return buildStory(in).OnCall.Number }

	if got := on(base()); got != 3 {
		t.Errorf("the last run: on call %02d, want 03", got)
	}
	in := base()
	in.Saves = map[string]time.Time{slug(c, 2): day(9, 6)}
	if got := on(in); got != 2 {
		t.Errorf("a later save: on call %02d, want 02", got)
	}
	in.Reveals = []store.Reveal{{Exercise: slug(c, 3), Level: 1, RevealedAt: day(9, 7)}}
	if got := on(in); got != 3 {
		t.Errorf("a later reveal: on call %02d, want 03", got)
	}
	// The exercise open in the IDE is the one worked in now, started or not.
	in.Here = slug(c, 9)
	if s := buildStory(in); s.OnCall.Number != 9 || !s.KeyOnRing || s.Open.N != 2 {
		t.Errorf("open in the IDE: on call %v, Key %v", s.OnCall, s.KeyOnRing)
	}
	// Ties go to the lower number.
	in = base()
	in.Progress = []store.Progress{started(c, 4, day(9, 5)), started(c, 2, day(9, 5))}
	if got := on(in); got != 2 {
		t.Errorf("a tie: on call %02d, want 02", got)
	}
	// Every started exercise passes: the lowest-numbered that does not.
	in.Progress = []store.Progress{passed(c, 1, day(9, 3)), passed(c, 2, day(9, 4)), passed(c, 4, day(9, 5))}
	if s := buildStory(in); s.OnCall.Number != 3 || s.Started != 3 || s.KeyOnRing {
		t.Errorf("all started pass: on call %v", s.OnCall)
	}
	// A tier-2 exercise started puts the Key on the ring, even while the
	// on-call exercise is in tier 1.
	in.Progress = append(in.Progress, started(c, 8, day(9, 1)), started(c, 3, day(9, 9)))
	if s := buildStory(in); !s.KeyOnRing || s.OnCall.Number != 3 || s.Open.N != 1 || s.Slab.State != "key" {
		t.Errorf("tier 2 started: Key %v, on call %v, slab %q", s.KeyOnRing, s.OnCall, s.Slab.State)
	}
}

// TestStoryHookBesideTag: T1 stamped and nothing of tier 2 started. The
// Key stays on its hook beside the tag, and the next exercise is 06.
func TestStoryHookBesideTag(t *testing.T) {
	c := storyCourse(t)
	in := storyInput{Course: c, User: ada, Now: now, Certs: certs(c, "Ada Lovelace", map[int]time.Time{1: day(9, 22)}, nil, false)}
	for n := 1; n <= 5; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(9, 10+n)))
	}
	s := buildStory(in)
	if s.KeyOnRing || s.OnCall.Number != 6 || s.Open.N != 2 || s.Slab.State != "hook" || s.Slab.MarkLabel != "" ||
		s.Slab.Button != "Start exercise 06" || s.Slab.Lead != "Your next exercise" {
		t.Fatalf("hook beside tag: Key %v, on call %v, slab %+v", s.KeyOnRing, s.OnCall, s.Slab)
	}
	if want := "Your ring: the stamped tag T1, Badge In, hangs on it. The Key hangs on its hook inside the dial, with its B0 tag. A dashed outline for T2, The Key, with 5 exercises to go. The tier dial: 1 of 10 stamped."; s.Ring.Title != want {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	svg := string(s.Ring.SVG)
	if !strings.Contains(svg, fmt.Sprintf("translate(%d %d) scale(1.6)", hookBesideTag.x, hookBesideTag.y)) || !strings.Contains(svg, `data-ring-part="tags"`) {
		t.Error("the hook is not beside the tag")
	}
	if !strings.HasPrefix(string(s.Ledger.Intro), "Each tool you write goes into the Ledger with you as its author, once") {
		t.Errorf("intro = %q", s.Ledger.Intro)
	}
}

// TestStoryReadyAndStamped: a tag is stamped only when its certificate
// is issued; a tier whose exercises all pass is ready to claim until then.
func TestStoryReadyAndStamped(t *testing.T) {
	c := storyCourse(t)
	in := adaInTier2(c)
	for n := 11; n <= 15; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(10, n)))
	}
	s := buildStory(in)
	t3 := s.Tiers[2]
	if !t3.IsReady() || t3.IsStamped() || s.Stamped != 1 || len(s.Ready) != 1 || t3.PlateWord() != "Ready to claim" ||
		t3.Progress() != "All 5 exercises pass." || t3.ClaimTitle() != "Claim the tier 3 certificate" ||
		t3.ClaimSummary() != "Tier 3, Hand Tools, covers exercises 11 to 15. All 5 pass." {
		t.Fatalf("tier 3 = %+v", t3)
	}
	items := s.Ring.Items
	// A ready tag has its own look, never the grey of a tier not started (VESPER.md 6.18).
	if len(items) != 3 || items[1].Text != "ready to claim" || items[1].Href != "/certificates" || items[1].N != 3 || items[1].TagClass() != "gpw-tag-ready" {
		t.Errorf("tag list = %+v", items)
	}
	if !strings.Contains(s.Ring.Title, "T3 is ready to claim.") || !strings.Contains(string(s.Ring.SVG), `class="gpw-sv-ready"`) {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	if s.CertsHead.Ready != "T3 ready to claim" || s.CertsHead.Tags != "1 of 10" {
		t.Errorf("certificates head = %+v", s.CertsHead)
	}
	// The Key's skyline: towers that pass are solid, whatever their tier.
	if want := "Towers 01 to 06 and 11 to 15 are solid: in the Ledger."; !strings.Contains(s.Skyline.Label, want) {
		t.Errorf("skyline label = %q", s.Skyline.Label)
	}
	if s.Ledger.Rest != "Tiers 3 to 10" {
		t.Errorf("rest = %q", s.Ledger.Rest)
	}
	// An earlier certificate stays valid, but it stamps no tag.
	in.Certs = certs(c, "Ada Lovelace", nil, nil, true)
	if s := buildStory(in); s.Stamped != 0 || s.Key.Earlier == nil || s.Key.Held != nil || s.Badge.TagText() != "Day 1" {
		t.Errorf("earlier only: stamped %d, Key %+v", s.Stamped, s.Key)
	}
}

// TestStoryReissue: "Issue again" corrects a certificate and stamps no new
// tag. The tag keeps the date of its first certificate, so a re-issued T1
// does not hang in front of T2.
func TestStoryReissue(t *testing.T) {
	c := storyCourse(t)
	in := storyInput{Course: c, User: ada, Now: now,
		Certs:       certs(c, "Ada Lovelace", map[int]time.Time{1: day(10, 21), 2: day(10, 12)}, nil, false),
		FirstIssued: map[string]time.Time{"tier1": day(9, 22), "tier2": day(10, 12)}}
	for n := 1; n <= 10; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(9, 10+n)))
	}
	s := buildStory(in)
	if s.Latest == nil || s.Latest.N != 2 {
		t.Fatalf("front tag = %v, want T2 (the tag stamped last)", s.Latest)
	}
	if got := s.Tiers[0].StampedText(); got != "Tag stamped 22\u00a0Sep" {
		t.Errorf("T1 = %q, want the first stamp's date", got)
	}
	if want := "Your ring: the stamped tag T2, The Key, hangs on it, with 1 earlier tag behind it."; !strings.HasPrefix(s.Ring.Title, want) {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	if items := s.Ring.Items; len(items) == 0 || items[0].N != 2 || items[0].Text != "stamped 12\u00a0Oct" {
		t.Errorf("tag list = %+v", items)
	}
	// Without a chain the current certificate's date stands.
	in.FirstIssued = nil
	if s := buildStory(in); s.Latest.N != 1 || s.Tiers[0].StampedText() != "Tag stamped 21\u00a0Oct" {
		t.Errorf("without FirstIssued: front %d, %q", s.Latest.N, s.Tiers[0].StampedText())
	}
}

// TestStoryReadyFold: tiers ready to claim share 1 row of the ring card's
// list, which never repeats the 10 tiers (VESPER.md 6.24). The Key on the
// badge replaces "Day 1".
func TestStoryReadyFold(t *testing.T) {
	c := storyCourse(t)
	anna := store.User{ID: 2, DisplayName: "Анна", CreatedAt: day(3, 1)}
	in := storyInput{Course: c, User: anna, Now: now, Certs: certs(c, "Анна", nil, nil, false)}
	for n := 1; n <= 50; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(10, 1)))
	}
	s := buildStory(in)
	if items := s.Ring.Items; len(items) != 1 || items[0].Fold != "T1 to T10" || items[0].Text != "ready to claim" || items[0].Href != "/certificates" {
		t.Errorf("tag list = %+v", items)
	}
	in.Certs = certs(c, "Анна", map[int]time.Time{2: day(10, 2), 3: day(10, 3), 4: day(10, 4)}, nil, false)
	if items := buildStory(in).Ring.Items; len(items) != 2 || items[0].N != 4 || items[1].Fold != "T1 and T5 to T10" {
		t.Errorf("tag list with T2 to T4 = %+v", items)
	}
	in.Certs = certs(c, "Анна", map[int]time.Time{1: day(10, 2)}, nil, false)
	if items := buildStory(in).Ring.Items; len(items) != 2 || items[1].Fold != "T2 to T10" {
		t.Errorf("tag list with T1 = %+v", items)
	}
	key := day(10, 5)
	in.Certs = certs(c, "Анна", nil, &key, false)
	s = buildStory(in)
	if s.Badge.TagText() != "Key" || s.Badge.Menu != "Holds the Key, the course certificate" || s.Readout.Ledger != "Анна, SRE" {
		t.Errorf("the Key without tags: badge %+v, readout %q", s.Badge, s.Readout.Ledger)
	}
}

// TestStoryLukasz is certificates.html: the first course passed and 5
// tags held, 01 to 08 in the Ledger, on call for 09, T9 ready to claim.
func TestStoryLukasz(t *testing.T) {
	c := storyCourse(t)
	name := "Łukasz Dvořák-Wiśniewska"
	in := storyInput{Course: c, User: store.User{DisplayName: name, CreatedAt: day(3, 1)}, Now: now,
		Certs: certs(c, name, map[int]time.Time{1: day(9, 18), 6: day(10, 20), 7: day(4, 2), 8: day(5, 9), 10: day(6, 26)}, nil, true)}
	for n := 26; n <= 50; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(3, 1)))
	}
	for n := 1; n <= 8; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(9, 1+n)))
	}
	in.Progress = append(in.Progress, started(c, 9, day(10, 21)))
	s := buildStory(in)
	if s.OnCall.Number != 9 || s.Latest.N != 6 || s.Highest.N != 10 || s.Stamped != 5 || len(s.Ready) != 1 || s.Ready[0].N != 9 {
		t.Fatalf("Łukasz: on call %v, latest %v, highest %v, stamped %d", s.OnCall, s.Latest, s.Highest, s.Stamped)
	}
	if b := s.Badge; b.Initials != "ŁD" || b.TagText() != "T10" || b.Label != name+" (ŁD), highest tag T10, The Key Turns. Account menu." {
		t.Errorf("badge = %+v", b)
	}
	if s.Readout.Ledger != name+", SRE" || s.Readout.MonthLine != "October, tier 2" {
		t.Errorf("readout = %+v", s.Readout)
	}
	if want := "Your ring: the stamped tag T6, Forensics, hangs on it, with 4 earlier tags behind it."; !strings.HasPrefix(s.Ring.Title, want) {
		t.Errorf("ring title = %q", s.Ring.Title)
	}
	h := s.CertsHead
	if h.Held != 5 || h.Kinds != 11 || h.Said != "5 of 11 certificates are yours and current." || h.Tags != "5 of 10" ||
		h.Ready != "T9 ready to claim" || h.Oldest != "2 Apr 2026, tier 7" || h.Newest != "20 Oct 2026, tier 6" {
		t.Errorf("certificates head = %+v", h)
	}
	if len(h.Years) != 2 || h.Years[0].Label != "Tiers 1 to 5: 1 held, 1 in progress, 3 not started" ||
		h.Years[1].Label != "Tiers 6 to 10: 4 held, 1 ready to claim" {
		t.Errorf("years = %+v", h.Years)
	}
	if h.KeyState != "In progress" || h.KeyCount != "33 of 50 in the Ledger. 17 to go: 09 to 25." {
		t.Errorf("the Key = %q %q", h.KeyState, h.KeyCount)
	}
	t1, t2, t3 := s.Tiers[0], s.Tiers[1], s.Tiers[2]
	if t1.CertLine() != name+" completed Tier 1, Badge In: you read the raw lines the dashboards miss." ||
		t2.PlateWord() != "In progress" || t2.Progress() != "3 of 5 in the Ledger. 2 to go: 09, 10." ||
		t2.LineSentence() != "You carry the Key, with types and errors that tell the truth." ||
		t3.PlateWord() != "Not started" || t3.Progress() != "0 of 5 in the Ledger." {
		t.Errorf("tiers: %q / %q %q / %q %q", t1.CertLine(), t2.PlateWord(), t2.Progress(), t3.PlateWord(), t3.Progress())
	}
	var cells []string
	for _, cell := range t2.Cells {
		cells = append(cells, cell.State)
	}
	if fmt.Sprint(cells) != "[passed passed passed oncall ]" {
		t.Errorf("tier 2 cells = %v", cells)
	}
}

// TestStoryAllPass: once every exercise passes the Calm Score falls to 71
// and the slab says the Key turned.
func TestStoryAllPass(t *testing.T) {
	c := storyCourse(t)
	all := map[int]time.Time{}
	in := storyInput{Course: c, User: ada, Now: now}
	for n := 1; n <= 50; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(10, 1)))
	}
	for n := 1; n <= 10; n++ {
		all[n] = day(10, 2).Add(time.Duration(n) * time.Minute)
	}
	in.Certs = certs(c, "Ada Lovelace", all, nil, false)
	s := buildStory(in)
	if !s.AllPass || s.OnCall != nil || s.Open != nil || !s.Key.Ready {
		t.Fatalf("all pass: %v %v %v", s.AllPass, s.OnCall, s.Open)
	}
	if s.Slab.State != "done" || s.Slab.Lead != "You turned the Key." || s.Slab.Heading != "Every tool in the Ledger has you as its author." ||
		s.Slab.Button != "Claim the course certificate" || s.Slab.Href != "/certificates" || s.Slab.Facts != nil || s.Slab.Exercise != nil {
		t.Errorf("slab = %+v", s.Slab)
	}
	if s.Calm.Score != "71" || !s.Calm.Moved || s.Calm.Say != "" || s.Calm.Scale != [3]string{"100", "75", "50"} ||
		s.Calm.Caption != "Lower, and true. Every pager in Meridian still works." || !strings.HasSuffix(s.Calm.Path, "L599.0 41.6") {
		t.Errorf("calm = %+v", s.Calm)
	}
	if !strings.HasSuffix(string(s.Ledger.Intro), "The clocks went back, and the night held.") || s.Ledger.Rest != "" {
		t.Errorf("ledger = %+v", s.Ledger)
	}
	if s.Readout.MonthLine != "The last week of October, tier 10" || s.Latest.N != 10 || s.Ring.Count != "10 of 10 tags" {
		t.Errorf("readout %q, latest %v", s.Readout.MonthLine, s.Latest)
	}
	if s.Skyline.Caption != "50 in the Ledger" || !strings.Contains(s.Skyline.Label, "All 50 towers are solid") {
		t.Errorf("skyline = %q %q", s.Skyline.Caption, s.Skyline.Label)
	}
	if s.CertsHead.KeyState != "Ready to claim" || s.CertsHead.KeyCount != "All 50 exercises pass." {
		t.Errorf("the Key = %q %q", s.CertsHead.KeyState, s.CertsHead.KeyCount)
	}
	key := day(10, 3)
	in.Certs = certs(c, "Ada Lovelace", all, &key, false)
	if s := buildStory(in); s.Slab.Button != "Open the course certificate" || s.Slab.Href != s.Key.Held.PDF || s.CertsHead.KeyState != "Held" {
		t.Errorf("the Key held: slab %+v", s.Slab)
	}
	// The capstone alone does not move the Calm Score, nor 49 of 50: the
	// slab would still name an exercise to do.
	in = storyInput{Course: c, User: ada, Now: now, Progress: []store.Progress{passed(c, 50, day(10, 1))}}
	if s := buildStory(in); s.Calm.Score != "99.2" || s.Calm.Moved {
		t.Errorf("after the capstone alone: %q", s.Calm.Score)
	}
	in.Progress = nil
	for n := 2; n <= 50; n++ {
		in.Progress = append(in.Progress, passed(c, n, day(10, 1)))
	}
	if s := buildStory(in); s.Calm.Score != "99.2" {
		t.Errorf("with 01 still to pass: %q", s.Calm.Score)
	}
}

func TestSkylineClasses(t *testing.T) {
	c := storyCourse(t)
	sky := buildSkyline(c, map[string]bool{slug(c, 1): true, slug(c, 2): true}, slug(c, 3))
	if len(sky.Years) != 2 || len(sky.Years[0].Tiers) != 5 || len(sky.Years[1].Tiers) != 5 {
		t.Fatalf("years = %+v", sky.Years)
	}
	g := sky.Years[0].Tiers[0]
	if g.Class() != "gpw-sky-g gpw-g5 gpw-g-top20" || g.PlateClass() != "gpw-sky-plate gpw-sky-plate-cur" {
		t.Errorf("tier 1 = %q %q", g.Class(), g.PlateClass())
	}
	var got []string
	for _, tw := range g.Towers {
		got = append(got, tw.Class())
	}
	want := []string{"gpw-tw gpw-h10 gpw-tw-on", "gpw-tw gpw-h10 gpw-tw-on", "gpw-tw gpw-h15 gpw-tw-cur", "gpw-tw gpw-h15", "gpw-tw gpw-h20 gpw-tw-roof"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("towers = %q", got)
	}
	if g := sky.Years[1].Tiers[4]; g.Class() != "gpw-sky-g gpw-g6 gpw-g-top60" || g.PlateClass() != "gpw-sky-plate" {
		t.Errorf("tier 10 = %q %q", g.Class(), g.PlateClass())
	}
	all := map[string]bool{}
	for _, e := range c.Tiers[1].Exercises {
		all[e.Slug] = true
	}
	if g := buildSkyline(c, all, "").Years[0].Tiers[1]; g.PlateClass() != "gpw-sky-plate gpw-sky-plate-on" {
		t.Errorf("a tier in the Ledger = %q", g.PlateClass())
	}
}

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{
		"Ada Lovelace":             "AL",
		"Łukasz Dvořák-Wiśniewska": "ŁD",
		"Ada King Lovelace":        "AK",
		"Анна":                     "А",
		"cher":                     "C",
		"  ":                       "?",
		"E\u0301mile Zola":         "E\u0301Z", // a decomposed É keeps its accent
	} {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNumberList(t *testing.T) {
	for _, tt := range []struct {
		nums []int
		and  bool
		want string
	}{
		{[]int{8, 9}, true, "08 and 09"},
		{[]int{9, 10}, false, "09, 10"},
		{[]int{7}, true, "07"},
		{[]int{9, 10, 11}, false, "09 to 11"},
		{[]int{1, 3, 5, 6, 7, 8, 9}, true, "01, 03 and 05 to 09"},
		{[]int{1, 2, 3, 4, 5, 6, 11, 12, 13, 14, 15}, true, "01 to 06 and 11 to 15"},
		{nil, true, ""},
	} {
		if got := numberList(tt.nums, tt.and); got != tt.want {
			t.Errorf("numberList(%v, %v) = %q, want %q", tt.nums, tt.and, got, tt.want)
		}
	}
	if got := rangeList([]int{3, 4, 5, 6, 7, 8, 9, 10}, "%d", true); got != "3 to 10" {
		t.Errorf("rangeList = %q", got)
	}
}

func TestFirstPassLine(t *testing.T) {
	want := map[int]string{
		1: "All green, and this time it is true.",
		2: "Green, and you earned it. HUSH had nothing to do with it.",
		3: "Into the Ledger. Author: you.",
		4: "All green, and this time it is true.",
		7: "All green, and this time it is true.",
	}
	for n, w := range want {
		if got := firstPassLine(n); got != w {
			t.Errorf("firstPassLine(%d) = %q", n, got)
		}
	}
	for n := 2; n <= 50; n++ {
		if firstPassLine(n) == firstPassLine(n-1) {
			t.Errorf("exercises %d and %d share a line", n-1, n)
		}
	}
}

func TestNotEligible(t *testing.T) {
	c := storyCourse(t)
	var passedSlugs []string
	for n := 1; n <= 7; n++ {
		passedSlugs = append(passedSlugs, slug(c, n))
	}
	passedSlugs = append(passedSlugs, slug(c, 10))
	for k, want := range map[cert.Kind]string{
		cert.TierKind(2): "Tier 2 needs every exercise to pass. 08 and 09 do not pass yet.",
		cert.TierKind(3): "Tier 3 needs every exercise to pass. 11 to 15 do not pass yet.",
		cert.KindCourse:  "The course certificate needs every exercise to pass. 08, 09 and 11 to 50 do not pass yet.",
	} {
		if got := notEligible(c, k, passedSlugs); got != want {
			t.Errorf("notEligible(%s) = %q", k, got)
		}
	}
	if got := notEligible(c, cert.TierKind(2), append(passedSlugs, slug(c, 8))); got != "Tier 2 needs every exercise to pass. 09 does not pass yet." {
		t.Errorf("notEligible with 1 missing = %q", got)
	}
}
