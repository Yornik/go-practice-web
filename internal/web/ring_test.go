package web

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/course/coursetest"
)

// The ring check of build.py (ring_check, ring_clash, _mask, _grow),
// ported: every part's footprint is rasterised on a grid of 1 unit, and
// 2 parts fail when they meet with less than 3 units between them,
// except where 1 is threaded on the other.

const gridN = 340

// gridWords is the number of 64-bit words in 1 row of the grid.
const gridWords = (gridN + 63) / 64

// grid is a bitmap of the drawing, 1 bit per unit square.
type grid [gridN * gridWords]uint64

func (g *grid) set(x, y int) { g[y*gridWords+x/64] |= 1 << (x % 64) }

func (g *grid) get(x, y int) bool { return g[y*gridWords+x/64]&(1<<(x%64)) != 0 }

func (g *grid) or(o *grid) {
	for i := range g {
		g[i] |= o[i]
	}
}

func (g *grid) meets(o *grid) bool {
	for i := range g {
		if g[i]&o[i] != 0 {
			return true
		}
	}
	return false
}

// rasterise fills each polygon even-odd at the centres of the unit
// squares inside its bounding box, as _mask does.
func rasterise(polys [][]pt) *grid {
	g := new(grid)
	for _, poly := range polys {
		minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range poly {
			minX, maxX, minY, maxY = min(minX, p.x), max(maxX, p.x), min(minY, p.y), max(maxY, p.y)
		}
		x0, x1 := max(int(minX)-1, 0), min(int(maxX)+2, gridN)
		y0, y1 := max(int(minY)-1, 0), min(int(maxY)+2, gridN)
		for y := y0; y < y1; y++ {
			cy := float64(y) + .5
			for x := x0; x < x1; x++ {
				cx := float64(x) + .5
				inside := false
				for i, a := range poly {
					b := poly[(i+1)%len(poly)]
					if a.y == b.y {
						continue
					}
					if cy >= min(a.y, b.y) && cy < max(a.y, b.y) && cx < a.x+(cy-a.y)*(b.x-a.x)/(b.y-a.y) {
						inside = !inside
					}
				}
				if inside {
					g.set(x, y)
				}
			}
		}
	}
	return g
}

// grow dilates the grid by k units in a square, as _grow does. _grow
// wraps at the edges (np.roll); no part comes within 4 units of an edge
// it could wrap to, so this one does not.
func grow(g *grid, k int) *grid {
	rows := new(grid)
	for y := range gridN {
		for d := -k; d <= k; d++ {
			if yy := y + d; yy >= 0 && yy < gridN {
				for w := range gridWords {
					rows[y*gridWords+w] |= g[yy*gridWords+w]
				}
			}
		}
	}
	out := new(grid)
	for y := range gridN {
		for x := range gridN {
			if !rows.get(x, y) {
				continue
			}
			for d := -k; d <= k; d++ {
				if xx := x + d; xx >= 0 && xx < gridN {
					out.set(xx, y)
				}
			}
		}
	}
	return out
}

// dialTrack is the dial's track, its end stops and bands: radius 129 to
// 143. Only the lanyard and a ghost's leader cross it.
var dialTrack = sync.OnceValue(func() *grid {
	g := new(grid)
	for y := range gridN {
		for x := range gridN {
			r := math.Hypot(float64(x)+.5-dialC.x, float64(y)+.5-dialC.y)
			if r >= dialR-7 && r <= dialR+7 {
				g.set(x, y)
			}
		}
	}
	return g
})

// ringTouch are the pairs that may meet: a tag, the Key and B0's cord are
// threaded on the ring, and the lanyard holds it.
var ringTouch = map[[2]string]bool{
	{"lanyard", "ring"}: true, {"ring", "tags"}: true, {"key", "ring"}: true, {"b0", "ring"}: true,
	{"dial", "lanyard"}: true, {"dial", "ghost"}: true,
}

func touches(a, b string) bool {
	if a > b {
		a, b = b, a
	}
	return ringTouch[[2]string{a, b}]
}

// masks caches the footprint of each distinct part, and its dilation.
type masks struct {
	mu    sync.Mutex
	plain map[string]*grid
	grown map[string]*grid
}

func (m *masks) of(p ringPart, pad int) (plain, grown *grid) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plain == nil {
		m.plain, m.grown = map[string]*grid{}, map[string]*grid{}
	}
	// The markup fixes the geometry: the same markup, the same footprint.
	key := p.name + "\x00" + p.svg
	if p.name == "dial" {
		key = "dial"
	}
	if _, ok := m.plain[key]; !ok {
		if p.name == "dial" {
			m.plain[key] = dialTrack()
		} else {
			m.plain[key] = rasterise(p.fp)
		}
	}
	gkey := fmt.Sprint(key, "\x00", pad)
	if _, ok := m.grown[gkey]; !ok {
		m.grown[gkey] = grow(m.plain[key], pad)
	}
	return m.plain[key], m.grown[gkey]
}

// clash returns the pairs of parts that meet with less than pad units
// between them, apart from the threaded ones.
func (m *masks) clash(parts []ringPart, pad int) []string {
	parts = append(slices.Clone(parts), ringPart{name: "dial"})
	var bad []string
	for i, a := range parts {
		_, ga := m.of(a, pad)
		for _, b := range parts[i+1:] {
			if touches(a.name, b.name) {
				continue
			}
			if pb, _ := m.of(b, pad); ga.meets(pb) {
				bad = append(bad, a.name+"+"+b.name)
			}
		}
	}
	return bad
}

// tiersOf loads the fixture of the course of 50: its tier names make the
// tags 1 or 2 lines high, as the real course's do.
func tiersOf(t *testing.T) []*course.Tier {
	t.Helper()
	c := coursetest.Load(t, coursetest.Fifty())
	if len(c.Tiers) != ringTiers {
		t.Fatalf("the fixture has %d tiers", len(c.Tiers))
	}
	return c.Tiers
}

// reachable lists every state of the drawing the store can produce with
// tier n open (0: none, every exercise passes): the Key on the ring with
// any tag in front, 0 to 2 edges behind it, and any open tier but the
// front one; and the Key on its hook. The Key is on its hook until an
// exercise of tier 2 starts, so then the open tier is 1 or 2, and only T1
// can be stamped (VESPER.md 16.1, 16.2).
func reachable(tiers []*course.Tier, n int) []ringState {
	var open *course.Tier
	if n > 0 {
		open = tiers[n-1]
	}
	out := []ringState{{KeyOnRing: true, Open: open, ToGo: 1}}
	for _, front := range tiers {
		if front == open {
			continue
		}
		for behind := range 3 {
			out = append(out, ringState{Stamped: behind + 1, Front: front, KeyOnRing: true, Open: open, ToGo: 1})
		}
	}
	if n == 1 || n == 2 {
		out = append(out, ringState{Open: open, ToGo: 5})
	}
	if n == 2 {
		out = append(out, ringState{Stamped: 1, Front: tiers[0], Open: open, ToGo: 5})
	}
	return out
}

// TestGhostLayout runs build.py's ring_layout() again, against every
// reachable state: for each tier, the angle nearest its segment's middle
// (0, ±3, ±6, ±9, ±11 degrees) and the largest radius from 112 down at
// which the ghost stays inside radius 126 and 4 units clear of all that
// can hang while the tier is open. The result must be ghostAt.
func TestGhostLayout(t *testing.T) {
	tiers := tiersOf(t)
	var m masks
	for n := 1; n <= ringTiers; n++ {
		group := new(grid)
		for _, st := range reachable(tiers, n) {
			st.Open = nil
			for _, p := range ringParts(st) {
				plain, _ := m.of(p, 4)
				group.or(plain)
			}
		}
		group = grow(group, 4)
		aMid := -90 + (n-1)*36 + 18
		var got ghostPlace
	search:
		for _, da := range []int{0, 3, -3, 6, -6, 9, -9, 11, -11} {
			a := aMid + da
			for r := 112; r > 58; r-- {
				p := ghostPart(tiers[n-1], 1, float64(a), float64(r))
				far := 0.0
				for _, q := range p.fp[0] {
					far = max(far, math.Hypot(q.x-dialC.x, q.y-dialC.y))
				}
				if far > 126 || group.meets(rasterise(p.fp)) {
					continue
				}
				got = ghostPlace{a, r}
				break search
			}
		}
		if got != ghostAt[n] {
			t.Errorf("T%d: the ghost fits at %v, ghostAt has %v", n, got, ghostAt[n])
		}
	}
}

// TestRingCheck is build.py's ring_check() over every reachable state,
// not only the 11 of the frames: no 2 parts meet with less than 3 units
// between them, apart from the threaded ones.
func TestRingCheck(t *testing.T) {
	tiers := tiersOf(t)
	var m masks
	states := 0
	for n := 0; n <= ringTiers; n++ {
		for _, st := range reachable(tiers, n) {
			states++
			if bad := m.clash(ringParts(st), 3); len(bad) > 0 {
				front, open := "none", "none"
				if st.Front != nil {
					front = st.Front.Tag()
				}
				if st.Open != nil {
					open = st.Open.Tag()
				}
				t.Errorf("%d stamped, %s in front, Key on the ring %v, %s open: %s meet",
					st.Stamped, front, st.KeyOnRing, open, strings.Join(bad, ", "))
			}
		}
	}
	if states < 300 {
		t.Errorf("only %d states checked", states)
	}
}

// TestHookBesideTagClear: right after the first claim the Key still
// hangs on its hook beside the T1 tag. The Key, its hook, B0 and the label
// stay 8 units clear of the tag, so the 2 never read as 1 object, and the
// label does not sit under the tag, where it would read as its caption.
func TestHookBesideTagClear(t *testing.T) {
	tiers := tiersOf(t)
	var hook, tags []pt
	var hookFP, tagsFP [][]pt
	for _, p := range ringParts(ringState{Stamped: 1, Front: tiers[0], Open: tiers[1], ToGo: 5}) {
		switch p.name {
		case "hook":
			hookFP = p.fp
			hook = p.fp[len(p.fp)-1]
		case "tags":
			tagsFP = p.fp
			tags = p.fp[len(p.fp)-1]
		}
	}
	if hookFP == nil || tagsFP == nil {
		t.Fatal("the state lacks the hook or the tag")
	}
	if grow(rasterise(tagsFP), 8).meets(rasterise(hookFP)) {
		t.Error("the hook comes within 8 units of the tag")
	}
	// The label's lower line against the front tag's outline.
	if hook[0].x < tags[1].x && hook[1].x > tags[0].x && hook[0].y >= tags[2].y {
		t.Errorf("the label at %v sits under the tag at %v", hook[0], tags[0])
	}
}

// TestRingSVG checks the drawing of the 2 dashboard frames: the parts in
// their places, in the frame's units.
func TestRingSVG(t *testing.T) {
	tiers := tiersOf(t)
	t2 := string(ringSVG(ringState{Stamped: 1, Front: tiers[0], KeyOnRing: true, Open: tiers[1], Done: 1, ToGo: 4,
		Bands: map[int]string{1: bandStamped}}, "ring-title", "Your ring"))
	for _, want := range []string{
		`<svg class="gpw-ring-draw" viewBox="0 0 340 340" role="img" aria-labelledby="ring-title"><title id="ring-title">Your ring</title>`,
		// T1 stamped: a band; T2 open: a dashed arc and a fifth of a band.
		`<path class="gpw-sv-amf" d="M177.3 30.2A140.0 140.0 0 0 1 246.2 52.6L241.9 59.3A132.0 132.0 0 0 0 176.9 38.2Z"/>`,
		`<path class="gpw-sv-am gpw-sv-dash" d="M255.6 64.3A136.0 136.0 0 0 1 297.0 121.3"/><path class="gpw-sv-amf" d="M258.1 61.2A140.0 140.0 0 0 1 269.0 71.0L263.3 76.7A132.0 132.0 0 0 0 253.1 67.4Z"/>`,
		`<text class="gpw-sv-t gpw-sv-t-dial" x="326.0" y="175.0" text-anchor="middle">T3</text>`,
		`<text class="gpw-sv-t gpw-sv-t-dial" x="121.8" y="26.6" text-anchor="middle">T1<tspan class="gpw-sv-z">0</tspan></text>`,
		`<g data-ring-part="lanyard"><path class="gpw-sv-lan" d="M165.0 0V95.0L168.5 106.0H171.5L175.0 95.0V0Z"/></g>`,
		`<g data-ring-part="key"><g transform="translate(196.8 151.5) rotate(-14.0) scale(1.15) translate(0 5)">`,
		`<path class="gpw-sv-cord" d="M143.8 117.6L117.8 133.6"/><g transform="translate(117.8 133.6) rotate(-5)">`,
		`<path class="gpw-sv-ctr" d="M170.0 93.0V177.0M127.0 136.0H213.0"/><circle class="gpw-sv-steel" cx="170.0" cy="136.0" r="31.0"/>`,
		`<g transform="translate(152.7 161.7)"><path class="gpw-sv-halo" d="M-28.0 -7.0H28.0V46.0L18.0 56.0H-28.0Z"/>`,
		`<text class="gpw-sv-t gpw-sv-t-stamp gpw-sv-t-plate" x="0" y="49" text-anchor="middle">BADGE IN</text></g><path class="gpw-sv-steel" d="M156.9 164.1A31.0 31.0 0 0 1 148.9 158.7"/>`,
		`<g data-ring-part="ghost"><path class="gpw-sv-am gpw-sv-dash" d="M273.6 94.8L266.7 96.6"/><g transform="translate(247.7 96.6)">`,
		`<text class="gpw-sv-t gpw-sv-t-gc gpw-sv-t-q" x="0" y="34" text-anchor="middle">4 TO GO</text>`,
	} {
		if !strings.Contains(t2, want) {
			t.Errorf("the drawing of T2 open lacks %s", want)
		}
	}
	if strings.Contains(t2, `data-ring-part="hook"`) || strings.Contains(t2, "+") {
		t.Error("the drawing of T2 open has a hook or a +N")
	}

	day1 := string(ringSVG(ringState{Open: tiers[0], ToGo: 5}, "ring-title", "Your ring"))
	for _, want := range []string{
		`<g data-ring-part="hook"><path class="gpw-sv-cord" d="M102 172L92 196"/><g transform="translate(116 160) scale(1.6) translate(0 5)">`,
		`<text class="gpw-sv-t gpw-sv-t-dial" x="138" y="244">ON ITS</text><text class="gpw-sv-t gpw-sv-t-dial" x="138" y="262">HOOK</text>`,
		`5 TO GO`,
	} {
		if !strings.Contains(day1, want) {
			t.Errorf("the drawing of day 1 lacks %s", want)
		}
	}
	for _, unwanted := range []string{`data-ring-part="key"`, `data-ring-part="b0"`, `data-ring-part="tags"`, "gpw-sv-amf"} {
		if strings.Contains(day1, unwanted) {
			t.Errorf("the drawing of day 1 has %s", unwanted)
		}
	}

	// A stack of 5 with T10 in front: 2 edges behind it, +4, the stamp in
	// 2 lines, and a ready tier as a keyline.
	stack := string(ringSVG(ringState{Stamped: 5, Front: tiers[9], KeyOnRing: true, Open: tiers[1], ToGo: 5,
		Bands: map[int]string{10: bandStamped, 3: bandReady}}, "r", "Your ring"))
	for _, want := range []string{
		// The edges clear the front tag's 3-unit halo by 3 units each.
		`<path class="gpw-sv-plate gpw-sv-stack" transform="translate(143.7 170.7)" d="M-28.0 -7.0H28.0V60.0L18.0 70.0H-28.0Z"/><path class="gpw-sv-plate gpw-sv-stack" transform="translate(146.7 167.7)"`,
		`<text class="gpw-sv-t gpw-sv-t-stamp gpw-sv-t-plate" x="-24.0" y="7">+4</text>`,
		`x="0" y="49" text-anchor="middle">THE KEY</text><text class="gpw-sv-t gpw-sv-t-stamp gpw-sv-t-plate" x="0" y="63" text-anchor="middle">TURNS</text>`,
		`<text class="gpw-sv-t gpw-sv-t-tag gpw-sv-t-plate" x="0" y="27" text-anchor="middle">T1<tspan class="gpw-sv-z">0</tspan></text>`,
		`<path class="gpw-sv-ready" d="M`,
	} {
		if !strings.Contains(stack, want) {
			t.Errorf("the drawing of a stack of 5 lacks %s", want)
		}
	}
	// CONTAINMENT: 1 word, set narrow and fitted to the plate.
	t7 := string(ringSVG(ringState{Stamped: 1, Front: tiers[6], KeyOnRing: true}, "r", "Your ring"))
	if !strings.Contains(t7, `<text class="gpw-sv-t gpw-sv-t-stamp gpw-sv-t-stamp-n gpw-sv-t-plate" x="0" y="49" text-anchor="middle" textLength="48" lengthAdjust="spacingAndGlyphs">CONTAINMENT</text>`) {
		t.Error("CONTAINMENT is not set narrow and fitted")
	}
	t6 := string(ringSVG(ringState{Stamped: 1, Front: tiers[5], KeyOnRing: true}, "r", "Your ring"))
	if !strings.Contains(t6, `gpw-sv-t-stamp-n gpw-sv-t-plate" x="0" y="49" text-anchor="middle">FORENSICS</text>`) {
		t.Error("FORENSICS is not set narrow")
	}
}
