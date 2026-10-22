package web

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/store"
)

// The story objects of VESPER.md 16: the ring and its tags, the Key and
// on call, the Ledger, the Calm Score and the staff badge. They are a
// view of the progress and certificate rows the store already holds: no
// table of their own, no randomness, and the only clock is the caller's
// (for "Today"). Every page view builds them again.

// storyInput is what the story is made of, as the store holds it.
type storyInput struct {
	Course   *course.Course
	User     store.User
	Progress []store.Progress
	Reveals  []store.Reveal
	Saves    map[string]time.Time // the newest code save per exercise
	Certs    []certStatus         // per kind: the current and the earlier certificate
	// FirstIssued is per kind the issue date of the first certificate in
	// the current one's chain of re-issues: the date its tag was stamped.
	FirstIssued map[string]time.Time
	Now         time.Time
	// Here is the exercise open in the IDE, or "". Opening it starts it
	// (the IDE's bootstrap request), so it counts as started and as the
	// exercise worked in last.
	Here string
}

// story is the learner's place in the story.
type story struct {
	Tiers []*tierTag // every tier, in order

	Passed  int  // exercises that pass: the tools in the Ledger
	Total   int  // exercises in the course
	Started int  // exercises started or passed
	Day1    bool // a new account: nothing started and no tag stamped
	AllPass bool // every exercise passes

	// OnCall is the exercise worked in last that does not pass; if every
	// started exercise passes, the lowest-numbered one that does not. nil
	// when every exercise passes (VESPER.md 16.2).
	OnCall *course.Exercise
	// Open is the tier of OnCall; nil when every exercise passes.
	Open *tierTag
	// KeyOnRing: an exercise of tier 2 or later is started or passes.
	// Before that the Key is on its hook.
	KeyOnRing bool

	Stamped int        // tags stamped: tiers with a current certificate
	Latest  *tierTag   // the most recently stamped tag, in front on the ring
	Highest *tierTag   // the highest stamped tag, on the staff badge
	Ready   []*tierTag // tiers ready to claim, in order

	Key keyState // the course certificate

	Ring      ringView
	Slab      slab
	Ledger    ledgerHead
	Calm      calm
	Badge     staffBadge
	Readout   readout
	Skyline   skyline
	CertsHead certsHead

	exercises map[string]exerciseState
	certs     []certStatus
}

// exerciseState is 1 exercise for the learner.
type exerciseState struct {
	Started       bool // started or passes
	Passed        bool
	ActiveSeconds int64
	Runs          int
	Hints         int // hint levels revealed
	Level         int // the highest ladder level revealed, 0 to 6
	PassedAt      time.Time
	worked        time.Time
}

// Exercise returns the learner's state of an exercise.
func (s *story) Exercise(slug string) exerciseState { return s.exercises[slug] }

// tierTag is 1 tier as a tag: on the ring, in the Ledger, on the
// certificates page.
type tierTag struct {
	*course.Tier
	State     string    // tagStamped, tagReady, tagOpen, tagStarted or tagNone
	StampedAt time.Time // the date of its first certificate, before any re-issue; zero unless stamped
	Cert      *certJSON // its current certificate; nil unless stamped
	Status    *certStatus
	Passed    int // its exercises that pass
	Started   int // its exercises started or passed
	Total     int
	Missing   []int // the numbers of its exercises that do not pass
	Cells     []tagCell
}

const (
	tagStamped = "stamped" // its certificate is issued
	tagReady   = "ready"   // every exercise passes, no certificate yet
	tagOpen    = "open"    // the tier of the on-call exercise
	tagStarted = "started" // an exercise is started or passes
	tagNone    = "none"
)

// tagCell is 1 exercise of a tier as a split-flap cell (VESPER.md 8.7).
type tagCell struct {
	*course.Exercise
	State string // "passed", "oncall" or ""
}

// IsStamped reports whether the tier's certificate is issued.
func (t *tierTag) IsStamped() bool { return t.State == tagStamped }

// IsReady reports whether every exercise passes and nothing is issued.
func (t *tierTag) IsReady() bool { return t.State == tagReady }

// IsOpen reports whether the tier holds the on-call exercise.
func (t *tierTag) IsOpen() bool { return t.State == tagOpen }

// ToGo is the number of its exercises that do not pass.
func (t *tierTag) ToGo() int { return t.Total - t.Passed }

// TagClass is the small tag's state class (VESPER.md 6.18): stamped
// solid, ready to claim in a 2 px amber keyline as on the dial and the
// certificates page, the open tier outlined in amber, the rest grey.
func (t *tierTag) TagClass() string {
	switch t.State {
	case tagStamped:
		return "gpw-tag-on"
	case tagReady:
		return "gpw-tag-ready"
	case tagOpen:
		return "gpw-tag-cur"
	}
	return "gpw-tag-off"
}

// TierValue is the tier as a readout value: "2, The Key".
func (t *tierTag) TierValue() string { return tierValue(t.Tier) }

func tierValue(t *course.Tier) string {
	if t.Name == "" {
		return itoa(t.N)
	}
	return itoa(t.N) + ", " + t.Name
}

// Topics is the line under the tier's name in the Ledger: its months,
// then its Go topics, each a sentence: "October. Types with methods, ….".
// The topics keep their code spans, so "`context`, an injected clock"
// reads as code, not as a sentence in lower case.
func (t *tierTag) Topics() template.HTML {
	theme := t.ThemeHTML
	if theme == "" {
		theme = template.HTML(template.HTMLEscapeString(t.Theme)) //nolint:gosec // escaped
	}
	if w := t.When(); w != "" {
		return template.HTML(template.HTMLEscapeString(w)) + ". " + theme + "." //nolint:gosec // escaped parts
	}
	return theme + "."
}

// InLedger is "1 of 5 in the Ledger".
func (t *tierTag) InLedger() string { return fmt.Sprintf("%d of %d in the Ledger", t.Passed, t.Total) }

// ToGoForTag is "4 to go for the tag", or "" when every exercise passes.
func (t *tierTag) ToGoForTag() string {
	if t.ToGo() == 0 {
		return ""
	}
	return fmt.Sprintf("%d to go for the tag", t.ToGo())
}

// StampedText is "Tag stamped 22 Sep", or "" when the tag is not stamped.
func (t *tierTag) StampedText() string {
	if !t.IsStamped() {
		return ""
	}
	return "Tag stamped " + dayMonth(t.StampedAt)
}

// PlateWord is the tier's state on the certificates page: "Held",
// "Ready to claim", "In progress" or "Not started".
func (t *tierTag) PlateWord() string {
	switch {
	case t.IsStamped():
		return "Held"
	case t.IsReady():
		return "Ready to claim"
	case t.Started > 0:
		return "In progress"
	}
	return "Not started"
}

// CertLine is the line of a held tier certificate: "Ada Lovelace
// completed Tier 1, Badge In: you read the raw lines the dashboards
// miss." "" when the tier holds none.
func (t *tierTag) CertLine() string {
	if t.Cert == nil {
		return ""
	}
	if l := t.Line(); l != "" {
		return t.Cert.Name + " completed " + t.Label() + ": " + l + "."
	}
	return t.Cert.Name + " completed " + t.Label() + "."
}

// Progress is what the certificates page says under a tier that is not
// held: "All 5 exercises pass.", "3 of 5 in the Ledger. 2 to go: 09,
// 10." or "0 of 5 in the Ledger.".
func (t *tierTag) Progress() string {
	switch {
	case t.IsStamped():
		return ""
	case t.IsReady():
		return fmt.Sprintf("All %d exercises pass.", t.Total)
	case t.Started > 0:
		return fmt.Sprintf("%s. %d to go: %s.", t.InLedger(), t.ToGo(), numberList(t.Missing, false))
	}
	return t.InLedger() + "."
}

// ClaimTitle is "Claim the tier 9 certificate".
func (t *tierTag) ClaimTitle() string { return fmt.Sprintf("Claim the tier %d certificate", t.N) }

// ClaimSummary is the claim dialog's line: "Tier 9, Clean Signal, covers
// exercises 40 to 44. All 5 pass."
func (t *tierTag) ClaimSummary() string {
	return fmt.Sprintf("%s, covers exercises %02d to %02d. All %d pass.", t.Label(), t.First, t.Last, t.Total)
}

// keyState is the course certificate, the Key (VESPER.md 8.7).
type keyState struct {
	Held    *certJSON // the current course certificate
	Earlier *certJSON // a course certificate of the first course
	Ready   bool      // every exercise passes and no current certificate
	Missing []int     // the numbers of the exercises that do not pass
}

// State is the Key's state on the certificates page.
func (k keyState) State(started int) string {
	switch {
	case k.Held != nil:
		return "Held"
	case k.Ready:
		return "Ready to claim"
	case started > 0:
		return "In progress"
	}
	return "Not started"
}

// buildStory derives the story from the store's rows.
func buildStory(in storyInput) *story {
	c := in.Course
	s := &story{Total: len(c.Exercises), exercises: make(map[string]exerciseState, len(c.Exercises)), certs: in.Certs}

	for _, p := range in.Progress {
		e := s.exercises[p.Exercise]
		e.Started, e.Passed = true, p.Passed()
		e.ActiveSeconds, e.Runs, e.PassedAt = p.ActiveSeconds, p.Runs, p.PassedAt
		e.worked = latest(p.StartedAt, p.LastRunAt, p.PassedAt, e.worked)
		s.exercises[p.Exercise] = e
	}
	for _, rv := range in.Reveals {
		e := s.exercises[rv.Exercise]
		if course.Level(rv.Level).IsHint() {
			e.Hints++
		}
		e.Level = max(e.Level, rv.Level)
		e.worked = latest(e.worked, rv.RevealedAt)
		s.exercises[rv.Exercise] = e
	}
	for slug, at := range in.Saves {
		e := s.exercises[slug]
		e.worked = latest(e.worked, at)
		s.exercises[slug] = e
	}
	if in.Here != "" {
		e := s.exercises[in.Here]
		e.Started, e.worked = true, in.Now
		s.exercises[in.Here] = e
	}

	// On call: the exercise worked in last that does not pass. Ties go to
	// the lower number, so the choice never depends on map order.
	var first, here *course.Exercise
	var lastAt time.Time
	for _, ex := range c.Exercises {
		e := s.exercises[ex.Slug]
		if e.Passed {
			s.Passed++
		}
		if e.Started {
			s.Started++
			if ex.Tier >= 2 {
				s.KeyOnRing = true
			}
		}
		if e.Passed {
			continue
		}
		if first == nil {
			first = ex
		}
		if e.Started && (s.OnCall == nil || e.worked.After(lastAt)) {
			s.OnCall, lastAt = ex, e.worked
		}
		if ex.Slug == in.Here {
			here = ex
		}
	}
	switch {
	case here != nil:
		// Also over a save of the same millisecond: it is open now.
		s.OnCall = here
	case s.OnCall == nil:
		s.OnCall = first
	}
	s.AllPass = s.Total > 0 && s.Passed == s.Total

	byKind := map[string]*certStatus{}
	for i := range in.Certs {
		byKind[in.Certs[i].Kind] = &in.Certs[i]
	}
	for _, t := range c.Tiers {
		tag := &tierTag{Tier: t, Total: len(t.Exercises), Status: byKind[string(cert.TierKind(t.N))]}
		for _, ex := range t.Exercises {
			e := s.exercises[ex.Slug]
			cell := tagCell{Exercise: ex}
			switch {
			case e.Passed:
				tag.Passed++
				cell.State = "passed"
			case s.OnCall == ex:
				cell.State = "oncall"
			}
			if !e.Passed {
				tag.Missing = append(tag.Missing, ex.Number)
			}
			if e.Started {
				tag.Started++
			}
			tag.Cells = append(tag.Cells, cell)
		}
		switch {
		case tag.Status != nil && tag.Status.Certificate != nil:
			tag.State, tag.Cert, tag.StampedAt = tagStamped, tag.Status.Certificate, tag.Status.Certificate.IssuedAt
			// A re-issue corrects a certificate; it stamps no new tag, so
			// the tag keeps its place on the ring and its date.
			if first := in.FirstIssued[tag.Status.Kind]; !first.IsZero() && first.Before(tag.StampedAt) {
				tag.StampedAt = first
			}
		case tag.Total > 0 && tag.Passed == tag.Total:
			tag.State = tagReady
		case s.OnCall != nil && s.OnCall.Tier == t.N:
			tag.State = tagOpen
		case tag.Started > 0:
			tag.State = tagStarted
		default:
			tag.State = tagNone
		}
		if s.OnCall != nil && s.OnCall.Tier == t.N {
			s.Open = tag
		}
		switch {
		case tag.IsStamped():
			s.Stamped++
			if s.Latest == nil || !tag.StampedAt.Before(s.Latest.StampedAt) {
				s.Latest = tag
			}
			s.Highest = tag
		case tag.IsReady():
			s.Ready = append(s.Ready, tag)
		}
		s.Tiers = append(s.Tiers, tag)
	}
	if k := byKind[string(cert.KindCourse)]; k != nil {
		s.Key.Held, s.Key.Earlier = k.Certificate, k.Earlier
	}
	s.Key.Ready = s.AllPass && s.Key.Held == nil
	for _, ex := range c.Exercises {
		if !s.exercises[ex.Slug].Passed {
			s.Key.Missing = append(s.Key.Missing, ex.Number)
		}
	}
	s.Day1 = s.Started == 0 && s.Stamped == 0

	s.Ring = s.ring()
	s.Slab = s.slab()
	s.Ledger = s.ledger(in.User, in.Now)
	s.Calm = calmScore(s.AllPass)
	s.Badge = newBadge(in.User.DisplayName, s.Highest, s.Key.Held != nil)
	s.Readout = s.readout(in.User.DisplayName)
	s.Skyline = s.skyline(c)
	s.CertsHead = s.certsHead(in.Certs)
	return s
}

func latest(ts ...time.Time) time.Time {
	var out time.Time
	for _, t := range ts {
		if t.After(out) {
			out = t
		}
	}
	return out
}

// ringView is the ring card (VESPER.md 6.24, 16.1).
type ringView struct {
	SVG       template.HTML // the drawing; "" for a course that is not 10 tiers
	Title     string        // the drawing's text alternative, its <title>
	Count     string        // "1 of 10 tags": the stamped tags
	Line      string        // the line under the drawing
	KeyOnRing bool          // the line's icon: the Key, or its hook
	Items     []ringItem    // the live tags (gpw-taglist)
}

// ringItem is 1 live tag in the list under the drawing.
type ringItem struct {
	*tierTag
	Text string    // "stamped 22 Sep", "ready to claim" or "4 to go"
	At   time.Time // the stamp's date, for a <time> element; zero for the others
	Href string    // "/certificates" for a tag ready to claim
	// Fold names 2 or more tiers ready to claim in 1 row, "T1 and T5 to
	// T10": the card lists live tags, and the tier list at the foot of the
	// page already holds all 10 (VESPER.md 6.24).
	Fold string
}

func (s *story) ring() ringView {
	v := ringView{Count: fmt.Sprintf("%d of %d tags", s.Stamped, len(s.Tiers)), KeyOnRing: s.KeyOnRing}
	if s.KeyOnRing {
		v.Line = "The Key is on your ring: you share the rotation with Wren."
	} else {
		v.Line = "The Key is on its hook. It clips onto your ring in tier 2."
	}
	if s.Latest != nil {
		v.Items = append(v.Items, ringItem{tierTag: s.Latest, Text: "stamped " + dayMonth(s.Latest.StampedAt), At: s.Latest.StampedAt})
	}
	switch len(s.Ready) {
	case 0:
	case 1:
		v.Items = append(v.Items, ringItem{tierTag: s.Ready[0], Text: "ready to claim", Href: "/certificates"})
	default:
		nums := make([]int, len(s.Ready))
		for i, t := range s.Ready {
			nums[i] = t.N
		}
		v.Items = append(v.Items, ringItem{tierTag: s.Ready[0], Text: "ready to claim", Href: "/certificates",
			Fold: rangeList(nums, "T%d", true)})
	}
	if s.Open != nil {
		v.Items = append(v.Items, ringItem{tierTag: s.Open, Text: fmt.Sprintf("%d to go", s.Open.ToGo())})
	}

	st := ringState{Stamped: s.Stamped, KeyOnRing: s.KeyOnRing, Bands: map[int]string{}}
	if s.Latest != nil {
		st.Front = s.Latest.Tier
	}
	if s.Open != nil {
		st.Open, st.Done, st.ToGo = s.Open.Tier, s.Open.Passed, s.Open.ToGo()
	}
	for _, t := range s.Tiers {
		switch {
		case t.IsStamped():
			st.Bands[t.N] = bandStamped
		case t.IsReady():
			st.Bands[t.N] = bandReady
		}
	}
	v.Title = s.ringTitle()
	if len(s.Tiers) == ringTiers {
		v.SVG = ringSVG(st, "ring-title", v.Title)
	}
	return v
}

// ringTitle says what hangs on the ring (VESPER.md 16.1).
func (s *story) ringTitle() string {
	var parts []string
	if s.Latest == nil {
		parts = append(parts, "Your ring: empty, on its lanyard.")
	} else {
		behind := ""
		if n := s.Stamped - 1; n > 0 {
			behind = fmt.Sprintf(", with %d earlier %s behind it", n, pluralWord(n, "tag", "tags"))
		}
		parts = append(parts, fmt.Sprintf("Your ring: the stamped tag %s, hangs on it%s.", tagName(s.Latest.Tier), behind))
	}
	switch {
	case !s.KeyOnRing:
		parts = append(parts, "The Key hangs on its hook inside the dial, with its B0 tag.")
	case s.Latest != nil:
		parts = append(parts, "The Key hangs beside it, with its B0 tag.")
	default:
		parts = append(parts, "The Key hangs on it, with its B0 tag.")
	}
	if s.Open != nil {
		parts = append(parts, fmt.Sprintf("A dashed outline for %s, with %d %s to go.",
			tagName(s.Open.Tier), s.Open.ToGo(), pluralWord(s.Open.ToGo(), "exercise", "exercises")))
	}
	if len(s.Ready) > 0 {
		tags := make([]string, len(s.Ready))
		for i, t := range s.Ready {
			tags[i] = t.Tag()
		}
		verb := "is"
		if len(tags) > 1 {
			verb = "are"
		}
		parts = append(parts, fmt.Sprintf("%s %s ready to claim.", andJoin(tags), verb))
	}
	if s.Stamped == 0 {
		parts = append(parts, "The tier dial: no tier stamped yet.")
	} else {
		parts = append(parts, fmt.Sprintf("The tier dial: %d of %d stamped.", s.Stamped, len(s.Tiers)))
	}
	return strings.Join(parts, " ")
}

// tagName is a tag in a sentence: "T1, Badge In".
func tagName(t *course.Tier) string {
	if t.Name == "" {
		return t.Tag()
	}
	return t.Tag() + ", " + t.Name
}

// slab is the on-call slab (VESPER.md 6.23, 16.2).
type slab struct {
	State     string           // "new", "hook", "key" or "done"
	Key       bool             // the Key as the mark and the Breaker 0 stripe; else the ring icon
	MarkLabel string           // "Day 1, floor 61", "Breaker 0, Hall 3" or ""
	Lead      string           // the bold label over the heading
	Exercise  *course.Exercise // the heading's exercise; nil when every exercise passes
	Heading   string           // the heading when every exercise passes
	Button    string
	Href      string
	Facts     []fact // nil when every exercise passes
}

// fact is a label over a value.
type fact struct{ Label, Value string }

func (s *story) slab() slab {
	if s.OnCall == nil {
		v := slab{State: "done", Key: true, MarkLabel: "Breaker 0, Hall 3", Lead: "You turned the Key.",
			Heading: "Every tool in the Ledger has you as its author.",
			Button:  "Claim the course certificate", Href: "/certificates"}
		if s.Key.Held != nil {
			v.Button, v.Href = "Open the course certificate", s.Key.Held.PDF
		}
		return v
	}
	ex := s.OnCall
	e := s.exercises[ex.Slug]
	v := slab{Exercise: ex, Lead: "Your next exercise", Href: "/learn/" + ex.Slug}
	switch {
	case s.KeyOnRing:
		v.State, v.Key, v.MarkLabel, v.Lead = "key", true, "Breaker 0, Hall 3", "You are on call for"
	case s.Day1:
		v.State, v.MarkLabel = "new", "Day 1, floor 61"
	default:
		v.State = "hook"
	}
	verb := "Start"
	if e.Started {
		verb = "Continue"
	}
	v.Button = fmt.Sprintf("%s exercise %02d", verb, ex.Number)
	active := "–"
	if e.Started {
		active = formatDuration(e.ActiveSeconds)
	}
	tier := itoa(ex.Tier)
	if s.Open != nil {
		tier = s.Open.TierValue()
	}
	v.Facts = []fact{{"Tier", tier}, {"Estimate", formatHours(ex.Hours)}, {"Active", active}, {"Revealed", revealedLevel(e.Level)}}
	return v
}

// revealedLevel is the highest ladder level revealed: "Nothing",
// "Hint 1", "Explanation".
func revealedLevel(l int) string {
	if l <= 0 {
		return "Nothing"
	}
	return course.Level(l).String()
}

// ledgerHead is the head of "Your page of the Ledger" (VESPER.md 8.6,
// 16.3).
type ledgerHead struct {
	Count   int           // the tools in the Ledger: exercises that pass
	Total   int           // the exercises of the course
	Said    string        // the hidden sentence of the count: "6 of 50 tools are in the Ledger."
	Intro   template.HTML // the sentence, or 2, under the title
	Day1    bool          // Intro is the new account's sentence
	AllPass bool          // Intro ends with "The clocks went back, and the night held."
	Active  string        // the strip: "7 h 40 min", or "None yet"
	Runs    int
	Hints   int       // hint levels revealed, over every exercise
	Since   string    // "2 Sep 2026", or "Today" on the day the account was made
	SinceAt time.Time // the account's creation, for a <time> element
	// Rest is the head over the tiers that are neither open nor stamped:
	// "Tiers 3 to 10"; "" when there are none.
	Rest string
}

func (s *story) ledger(u store.User, now time.Time) ledgerHead {
	l := ledgerHead{Count: s.Passed, Total: s.Total, Day1: s.Day1, AllPass: s.AllPass, SinceAt: u.CreatedAt}
	verb := "are"
	if s.Passed == 1 {
		verb = "is"
	}
	l.Said = fmt.Sprintf("%d of %d tools %s in the Ledger.", s.Passed, s.Total, verb)
	switch {
	case s.Day1:
		l.Intro = "Each tool you write goes into the Ledger with you as its author. Your ring is empty, and the Key is on its hook."
	case s.AllPass:
		l.Intro = "Each tool you write goes into the Ledger with you as its author, once <code>go test -race</code> passes. The clocks went back, and the night held."
	default:
		l.Intro = "Each tool you write goes into the Ledger with you as its author, once <code>go test -race</code> passes."
	}
	var active int64
	for _, e := range s.exercises {
		active += e.ActiveSeconds
		l.Runs += e.Runs
		l.Hints += e.Hints
	}
	l.Active = "None yet"
	if active > 0 {
		l.Active = formatDuration(active)
	}
	l.Since = shortDate(u.CreatedAt)
	if !u.CreatedAt.IsZero() && sameDay(u.CreatedAt, now) {
		l.Since = "Today"
	}
	var rest []int
	for _, t := range s.Tiers {
		if !t.IsOpen() && !t.IsStamped() {
			rest = append(rest, t.N)
		}
	}
	switch len(rest) {
	case 0:
	case 1:
		l.Rest = fmt.Sprintf("Tier %d", rest[0])
	default:
		l.Rest = "Tiers " + rangeList(rest, "%d", true)
	}
	return l
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

// calm is the Calm Score (VESPER.md 6.21, 16.4): the same for every
// learner until every exercise passes, and it never ticks. It waits for
// all 50, not for exercise 50 alone: its last state is the story's last
// beat, and a learner can pass 50 before 09 (the slab then still names 09).
type calm struct {
	Score   string    // "99.2", or "71" after every exercise passes
	Moved   bool      // the line moves: after every exercise passes
	Scale   [3]string // the y scale, top to bottom
	Path    string    // the line of the dashboard's chart, in its 600 by 72 box
	Say     string    // HUSH's line; "" after every exercise passes
	Caption string
}

// calmMoved is the 30 nights after the capstone: a static line with real
// ups and downs on a scale of 50 to 100, ending at 71. The frame drew it
// once from a seeded random walk (components.html); it is fixed here.
const calmMoved = "M1.0 36.8L20.9 39.0L40.9 30.1L60.8 38.8L80.7 44.6L100.7 53.6L120.6 49.4L140.5 43.9L160.5 35.9L180.4 30.4L200.3 21.9L220.3 25.3L240.2 22.9L260.1 28.6L280.1 33.3L300.0 30.5L319.9 24.5L339.9 21.8L359.8 13.4L379.7 16.0L399.7 10.4L419.6 8.0L439.5 15.0L459.5 13.9L479.4 21.9L499.3 30.5L519.3 33.5L539.2 29.0L559.1 22.6L579.1 18.1L599.0 41.6"

func calmScore(allPass bool) calm {
	if allPass {
		return calm{Score: "71", Moved: true, Scale: [3]string{"100", "75", "50"}, Path: calmMoved,
			Caption: "Lower, and true. Every pager in Meridian still works."}
	}
	return calm{Score: "99.2", Scale: [3]string{"99.4", "99.2", "99.0"}, Path: "M1 36H599",
		Say: "All 40" + nbsp + "000 customers are calm. Please rest.", Caption: "It said 99.2 on your first day too."}
}

// staffBadge is the account button and the account page's card
// (VESPER.md 6.5, 16.6).
type staffBadge struct {
	Name     string
	Initials string       // "AL", "ŁD"
	Tag      *course.Tier // the highest stamped tag; nil before the first and with the Key
	Key      bool         // the course certificate is held: it outranks every tag
	Known    bool         // the tags are known; false when they could not load
	Label    string       // the button's accessible name
	Menu     string       // the menu's line under the name: "Highest tag T1, Badge In"
}

// TagText is the badge's second cell: "T1", "Key" or "Day 1".
func (b staffBadge) TagText() string {
	switch {
	case b.Key:
		return "Key"
	case b.Tag != nil:
		return b.Tag.Tag()
	case b.Known:
		return "Day 1"
	}
	return ""
}

// newBadge is the staff badge (VESPER.md 16.6). The Key covers all 50
// exercises, so a learner who holds it never reads "Day 1", also without a
// tier tag.
func newBadge(name string, highest *tierTag, key bool) staffBadge {
	b := staffBadge{Name: name, Initials: initials(name), Known: true}
	who := fmt.Sprintf("%s (%s)", name, b.Initials)
	switch {
	case key:
		b.Key = true
		b.Menu = "Holds the Key, the course certificate"
		b.Label = who + ", holds the Key, the course certificate. Account menu."
	case highest != nil:
		b.Tag = highest.Tier
		b.Menu = "Highest tag " + tagName(highest.Tier)
		b.Label = fmt.Sprintf("%s, highest tag %s. Account menu.", who, tagName(highest.Tier))
	default:
		b.Menu = "Day 1, no tags yet"
		b.Label = who + ", day 1, no tags yet. Account menu."
	}
	return b
}

// unknownBadge is the badge when the tags could not load: the initials
// and the name, no tag rather than a wrong one.
func unknownBadge(name string) staffBadge {
	b := staffBadge{Name: name, Initials: initials(name)}
	b.Label = fmt.Sprintf("%s (%s). Account menu.", name, b.Initials)
	return b
}

// readout is the readout strip of the app pages (VESPER.md 6.5).
type readout struct {
	Ledger    string // "Ada Lovelace, junior SRE"
	Month     string // "October": the open tier's month in Meridian
	Tier      int    // 2: "tier 2" is set amber
	MonthLine string // "October, tier 2", the plain text
	KeyOnRing bool   // the Rotation: "On call, with Wren" ("On call" amber)
	Rotation  string // the Rotation's plain text; "The Key is on its hook" before
}

func (s *story) readout(name string) readout {
	// Tiers 1 to 5 are the first months as a junior; a tag of year 2 makes
	// the learner an SRE (the frames: Ada with T1 is a junior SRE,
	// Łukasz with T10 an SRE).
	role := "junior SRE"
	if s.Highest != nil && s.Highest.Year() >= 2 || s.Key.Held != nil {
		role = "SRE"
	}
	r := readout{Ledger: name + ", " + role, KeyOnRing: s.KeyOnRing}
	t := s.Open
	if t == nil && len(s.Tiers) > 0 {
		t = s.Tiers[len(s.Tiers)-1] // every exercise passes: the last week of October
	}
	if t != nil {
		r.Month, r.Tier = t.Month(), t.N
		r.MonthLine = fmt.Sprintf("tier %d", t.N)
		if r.Month != "" {
			r.MonthLine = r.Month + ", " + r.MonthLine
		}
	}
	r.Rotation = "The Key is on its hook"
	if s.KeyOnRing {
		r.Rotation = "On call, with Wren"
	}
	return r
}

func (s *story) skyline(c *course.Course) skyline {
	on := map[string]bool{}
	var passed []int
	others, othersStarted := 0, false
	for _, ex := range c.Exercises {
		e := s.exercises[ex.Slug]
		switch {
		case e.Passed:
			on[ex.Slug] = true
			passed = append(passed, ex.Number)
		case ex != s.OnCall:
			others++
			othersStarted = othersStarted || e.Started
		}
	}
	cur := ""
	if s.OnCall != nil {
		cur = s.OnCall.Slug
	}
	sky := buildSkyline(c, on, cur)
	sky.CurWord = "Next"
	if s.KeyOnRing {
		sky.CurWord = "On call"
	}

	label := []string{fmt.Sprintf("Your skyline: %d towers, 1 per exercise, as tall as its estimate.", s.Total)}
	switch {
	case s.AllPass:
		label = append(label, fmt.Sprintf("All %d towers are solid: in the Ledger.", s.Total))
	case len(passed) == 1:
		label = append(label, fmt.Sprintf("Tower %02d is solid: in the Ledger.", passed[0]))
	case len(passed) > 1:
		label = append(label, fmt.Sprintf("Towers %s are solid: in the Ledger.", numberList(passed, true)))
	}
	if s.OnCall != nil {
		if s.KeyOnRing {
			label = append(label, fmt.Sprintf("Tower %02d is amber: you are on call for it.", s.OnCall.Number))
		} else {
			label = append(label, fmt.Sprintf("Tower %02d is amber: it is next.", s.OnCall.Number))
		}
	}
	what := "not started"
	if othersStarted {
		what = "not in the Ledger yet"
	}
	if others > 0 {
		// The legend says what the label says.
		sky.OffWord = strings.ToUpper(what[:1]) + what[1:]
	}
	switch {
	case others == 1:
		label = append(label, "The other tower is an outline: "+what+".")
	case others > 1:
		label = append(label, fmt.Sprintf("The other %d are outlines: %s.", others, what))
	}
	sky.Label = strings.Join(label, " ")

	switch {
	case s.OnCall == nil:
		sky.Caption = fmt.Sprintf("%d in the Ledger", s.Passed)
	case s.KeyOnRing:
		sky.Caption = fmt.Sprintf("%d in the Ledger, %02d on call", s.Passed, s.OnCall.Number)
	default:
		sky.Caption = fmt.Sprintf("%d in the Ledger, %02d is next", s.Passed, s.OnCall.Number)
	}
	return sky
}

// certsHead is the head of the certificates page (VESPER.md 8.7).
type certsHead struct {
	Held   int    // certificates held and current, tiers and course
	Kinds  int    // the kinds of certificate: 11
	Said   string // the hidden sentence of the count
	Tags   string // "5 of 10": the stamped tags
	Ready  string // "T9 ready to claim"; "" when no tier is
	Oldest string // "2 Apr 2026, tier 7"; "" without a certificate
	Newest string // "20 Oct 2026, tier 6"
	Years  []certYear
	// KeyCount is the Key's line: "33 of 50 in the Ledger. 17 to go: 09
	// to 25." or "All 50 exercises pass."
	KeyCount string
	KeyState string // "Held", "Ready to claim", "In progress" or "Not started"
}

// certYear is 1 year of the tag rail.
type certYear struct {
	N     int
	Label string // "Tiers 1 to 5: 1 held, 1 in progress, 3 not started"
	Tiers []*tierTag
}

func (s *story) certsHead(certs []certStatus) certsHead {
	h := certsHead{Kinds: len(certs), Tags: fmt.Sprintf("%d of %d", s.Stamped, len(s.Tiers))}
	var oldest, newest *certJSON
	var oldestKind, newestKind string
	for _, k := range certs {
		c := k.Certificate
		if c == nil {
			continue
		}
		h.Held++
		if oldest == nil || c.IssuedAt.Before(oldest.IssuedAt) {
			oldest, oldestKind = c, k.Kind
		}
		if newest == nil || !c.IssuedAt.Before(newest.IssuedAt) {
			newest, newestKind = c, k.Kind
		}
	}
	verb := "are"
	if h.Held == 1 {
		verb = "is"
	}
	h.Said = fmt.Sprintf("%d of %d certificates %s yours and current.", h.Held, h.Kinds, verb)
	if oldest != nil {
		h.Oldest = shortDate(oldest.IssuedAt) + ", " + kindWords(oldestKind)
		h.Newest = shortDate(newest.IssuedAt) + ", " + kindWords(newestKind)
	}
	if len(s.Ready) > 0 {
		tags := make([]string, len(s.Ready))
		for i, t := range s.Ready {
			tags[i] = t.Tag()
		}
		h.Ready = andJoin(tags) + " ready to claim"
	}
	for _, t := range s.Tiers {
		if n := len(h.Years); n == 0 || h.Years[n-1].N != t.Year() {
			h.Years = append(h.Years, certYear{N: t.Year()})
		}
		y := &h.Years[len(h.Years)-1]
		y.Tiers = append(y.Tiers, t)
	}
	for i := range h.Years {
		y := &h.Years[i]
		counts := map[string]int{}
		for _, t := range y.Tiers {
			counts[t.PlateWord()]++
		}
		var parts []string
		for _, w := range []string{"Held", "Ready to claim", "In progress", "Not started"} {
			if n := counts[w]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, strings.ToLower(w)))
			}
		}
		first, last := y.Tiers[0].N, y.Tiers[len(y.Tiers)-1].N
		span := fmt.Sprintf("Tiers %d to %d", first, last)
		if first == last {
			span = fmt.Sprintf("Tier %d", first)
		}
		y.Label = span + ": " + strings.Join(parts, ", ")
	}
	h.KeyState = s.Key.State(s.Started)
	if s.AllPass {
		h.KeyCount = fmt.Sprintf("All %d exercises pass.", s.Total)
	} else {
		h.KeyCount = fmt.Sprintf("%d of %d in the Ledger. %d to go: %s.", s.Passed, s.Total, len(s.Key.Missing), numberList(s.Key.Missing, false))
	}
	return h
}

// kindWords names a kind after a date: "tier 7", or "the Key".
func kindWords(kind string) string {
	if n, ok := cert.Kind(kind).Tier(); ok {
		return fmt.Sprintf("tier %d", n)
	}
	return "the Key"
}

// numberList writes exercise numbers for a sentence: runs of 3 or more as
// "09 to 25", the rest 1 by 1. With and, the last 2 items join with
// "and" ("08 and 09", "01, 03 and 05 to 09"); without, all with commas
// ("09, 10"), as a list after a colon.
func numberList(nums []int, and bool) string { return rangeList(nums, "%02d", and) }

// rangeList is numberList in any number format: "%d" for tiers.
func rangeList(nums []int, format string, and bool) string {
	items := runs(nums, format)
	if and {
		return andJoin(items)
	}
	return strings.Join(items, ", ")
}

func runs(nums []int, format string) []string {
	var items []string
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		if j-i >= 2 {
			items = append(items, fmt.Sprintf(format+" to "+format, nums[i], nums[j]))
		} else {
			for k := i; k <= j; k++ {
				items = append(items, fmt.Sprintf(format, nums[k]))
			}
		}
		i = j + 1
	}
	return items
}

// andJoin joins "a", "b" and "c" as "a, b and c".
func andJoin(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// dayMonth is a date in a tag's line: "22 Sep". The year is the story's:
// the tag list and the Ledger show this year's work.
func dayMonth(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2" + nbsp + "Jan")
}

// firstPassLines are the story lines of a first pass (VESPER.md 16.8).
var firstPassLines = [3]string{
	"All green, and this time it is true.",
	"Green, and you earned it. HUSH had nothing to do with it.",
	"Into the Ledger. Author: you.",
}

// firstPassLine is the line of exercise n's first pass: the 3 lines rotate
// by exercise number, so 2 exercises in a row never share 1.
func firstPassLine(n int) string {
	if n < 1 {
		return firstPassLines[0]
	}
	return firstPassLines[(n-1)%len(firstPassLines)]
}

// initials are the staff badge's letters: the first letters of the first
// 2 words of the name, upper case, with any combining marks that follow
// them ("ŁD" for "Łukasz Dvořák-Wiśniewska"); 1 letter for a name of 1
// word.
func initials(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return "?"
	}
	var b strings.Builder
	for _, w := range words[:min(len(words), 2)] {
		r, size := utf8.DecodeRuneInString(w)
		b.WriteString(strings.ToUpper(string(r)))
		for _, m := range w[size:] {
			if !unicode.Is(unicode.Mn, m) {
				break
			}
			b.WriteRune(m)
		}
	}
	return b.String()
}

// loadStory reads the learner's rows and builds the story. here is the
// exercise open in the IDE, or "".
func (s *Server) loadStory(ctx context.Context, u store.User, here string) (*story, error) {
	in := storyInput{Course: s.cfg.Course, User: u, Now: s.now(), Here: here}
	var err error
	if in.Progress, err = s.cfg.Store.UserProgress(ctx, u.ID); err != nil {
		return nil, err
	}
	if in.Reveals, err = s.cfg.Store.UserReveals(ctx, u.ID); err != nil {
		return nil, err
	}
	if in.Saves, err = s.cfg.Store.LastSaves(ctx, u.ID); err != nil {
		return nil, err
	}
	held, err := s.cfg.Store.UserCertificates(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if in.FirstIssued, err = s.cfg.Store.FirstIssued(ctx, u.ID); err != nil {
		return nil, err
	}
	var passed []string
	for _, p := range in.Progress {
		if p.Passed() {
			passed = append(passed, p.Exercise)
		}
	}
	in.Certs = s.certificateStatusFrom(passed, held)
	return buildStory(in), nil
}
