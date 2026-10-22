package web

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/course"
)

// The ring drawing of the dashboard (VESPER.md 16.1): a technical drawing
// in its own units, viewBox 340 by 340, ported from ring_parts and
// ring_svg in .design/vesper-v2/tools/build.py. The tier dial is the
// progress; the steel ring hangs inside it on its lanyard and holds the
// stamped tags and the Key. Every part records its footprint, and the
// tests rasterise them for every state the store can produce: no 2 parts
// meet, except where 1 is threaded on the other. Angles are SVG degrees:
// 0 is 3 o'clock, 90 is 6 o'clock.

// ringTiers is the number of dial segments. The geometry, and the table of
// ghost places, is drawn for the course of 10 tiers.
const ringTiers = 10

type pt struct{ x, y float64 }

var (
	dialC = pt{170, 170}
	ringC = pt{170, 136}
)

const (
	dialR = 136.0
	ringR = 31.0 // wider than half the tag (28), so the ring never reads as a shackle

	tagW, tagTop = 56.0, -7.0 // a stamped tag: 56 wide, its hole 7 under its top edge
	tagAt        = 124.0      // the stamped tags hang on the ring's lowest arc

	ghostW, ghostTop, ghostH = 38.0, -6.0, 40.0

	keyS, keyAt, keyTurn = 1.15, 30.0, -14.0 // the Key on the ring: scale, where its bow sits, its turn

	b0At = 215.0 // B0 hangs on its own cord from the ring at 10 o'clock
)

// stackAt is how far down and left each earlier tag hangs behind the
// front one. The front tag's halo reaches 3 units out, so at build.py's 3
// units the first edge hid under it. At 6 and 9 each edge shows 3 units.
var stackAt = [3]float64{0, 6, 9}

var b0Cord = pt{-26, 16}

// keyBits are the Key's 3 bits in its own units: from, to and how far
// they stand out (VESPER.md 6.28).
var keyBits = [3][3]float64{{36, 41, 11}, {44, 48.5, 8}, {52, 60, 11}}

// keyPath is the Key's outline with its hole (fill-rule evenodd): the
// shaft points down and the bits stand to the right. build.py's KEY_DV.
const keyPath = "M3 11.62L3 36L11 36L11 41L3 41L3 44L8 44L8 48.5L3 48.5L3 52L11 52L11 60L-3 60L-3 11.62A12 12 0 1 1 3 11.62ZM5 0A5 5 0 1 0 -5 0A5 5 0 1 0 5 0Z"

// ghostPlace is where the open tier's ghost tag sits: the angle on the dial
// and the radius of its centre, inside the tier's own segment.
type ghostPlace struct{ angle, radius int }

// ghostAt is build.py's GHOST_AT: for each tier the angle nearest its
// segment's middle and the largest radius at which the ghost and its
// leader stay inside the dial and 4 units clear of what hangs on the ring.
// build.py's ring_layout() clears only the state of the frames, in which
// the tiers before the open one are stamped. The site's learner can stamp
// any tiers and be on call in any other, so TestGhostLayout runs the same
// search against every state the store can produce. That moves 1 ghost:
// T6 from 108 degrees, 97 to 99 degrees, 99, because 3 tags with a
// 2-line stamp in front (T3 after T1 and T2), their edges at stackAt,
// reach its frame place.
var ghostAt = [ringTiers + 1]ghostPlace{
	1: {-61, 96}, 2: {-36, 96}, 3: {0, 104}, 4: {36, 101}, 5: {72, 100},
	6: {99, 99}, 7: {144, 96}, 8: {171, 102}, 9: {227, 96}, 10: {252, 97},
}

// hookPlace is where the Key's hook is drawn while the Key is not on the
// ring, inside the dial at 7 to 8 o'clock: the hook at x, y, the
// "ON ITS / HOOK" label at tx, ty, and B0's hole bx, by from the hook,
// turned brot degrees.
type hookPlace struct{ x, y, tx, ty, bx, by, brot int }

var (
	// hookDay1 is the hook of an empty ring (dashboard-day1.html).
	hookDay1 = hookPlace{116, 160, 138, 244, -24, 36, -4}
	// hookBesideTag is the hook while the T1 tag hangs on the ring and no
	// exercise of tier 2 is started yet: the dashboard right after the
	// first claim. No frame draws that state. At hookDay1 the Key would
	// cross the tag, so it hangs further left, 8 units or more clear of
	// the tag (TestHookBesideTagClear), and B0 hangs closer under its bow.
	// Under the tag the label read as the tag's caption; it stands above
	// the hook's wall plate.
	hookBesideTag = hookPlace{98, 138, 80, 99, -18, 36, -4}
)

// ringState is what the drawing shows.
type ringState struct {
	Stamped   int          // the tags on the ring
	Front     *course.Tier // the tag in front: the most recently stamped; nil without tags
	KeyOnRing bool         // the Key hangs on the ring; else on its hook inside the dial
	Open      *course.Tier // the open tier's ghost tag; nil when every exercise passes
	Done      int          // the open tier's exercises that pass: its share of the dial
	ToGo      int          // the open tier's exercises that do not pass
	// Bands is the dial's state per tier number: bandStamped or
	// bandReady. The open tier comes from Open.
	Bands map[int]string
}

const (
	bandStamped = "stamped"
	bandReady   = "ready"
)

// hook is where the Key's hook hangs in this state.
func (st ringState) hook() hookPlace {
	if st.Stamped > 0 {
		return hookBesideTag
	}
	return hookDay1
}

// ringPart is 1 part of the drawing: its markup and its footprint.
type ringPart struct {
	name string
	svg  string
	fp   [][]pt
}

func polar(a, r float64, o pt) pt {
	rad := a * math.Pi / 180
	return pt{o.x + r*math.Cos(rad), o.y + r*math.Sin(rad)}
}

// xf turns, scales and moves a polygon: rot in degrees, then sc, then x, y.
func xf(pts []pt, x, y, rot, sc float64) []pt {
	c, s := math.Cos(rot*math.Pi/180), math.Sin(rot*math.Pi/180)
	out := make([]pt, len(pts))
	for i, p := range pts {
		out[i] = pt{x + sc*(p.x*c-p.y*s), y + sc*(p.x*s+p.y*c)}
	}
	return out
}

func circlePoly(cx, cy, r float64, n int) []pt {
	out := make([]pt, n)
	for k := range out {
		a := 2 * math.Pi * float64(k) / float64(n)
		out[k] = pt{cx + r*math.Cos(a), cy + r*math.Sin(a)}
	}
	return out
}

// segPoly is a straight line of width w as a polygon.
func segPoly(a, b pt, w float64) []pt {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		l = 1
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	return []pt{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}}
}

func tagOutline(h float64) []pt {
	return []pt{{-tagW / 2, tagTop}, {tagW / 2, tagTop}, {tagW / 2, h - 10}, {tagW/2 - 10, h}, {-tagW / 2, h}}
}

func ghostOutline() []pt {
	w2 := ghostW / 2
	return []pt{{-w2, ghostTop}, {w2, ghostTop}, {w2, ghostH - 8}, {w2 - 8, ghostH}, {-w2, ghostH}}
}

// keyOutline is the Key's silhouette with its hole at 0, 0, moved 5 down
// as the drawing hangs it: the shaft with its bits, and the bow.
func keyOutline() (body, bow []pt) {
	j := 11.62 // where the shaft's sides meet the bow: √(12² − 3²), rounded as the path is
	body = []pt{{3, j}}
	for _, b := range keyBits {
		body = append(body, pt{3, b[0]}, pt{b[2], b[0]}, pt{b[2], b[1]}, pt{3, b[1]})
	}
	body = append(body[:len(body)-1], pt{-3, 60}, pt{-3, j})
	bow = circlePoly(0, 0, 12, 28)
	for i := range body {
		body[i].y += 5
	}
	for i := range bow {
		bow[i].y += 5
	}
	return body, bow
}

// tagHeight is the plate's height: 63 units, 77 for a stamp of 2 lines
// (from the hole: 7 up, 56 or 70 down).
func tagHeight(t *course.Tier) float64 {
	if len(t.StampLines()) > 1 {
		return 70
	}
	return 56
}

// f1 formats a coordinate as the drawing writes it: 1 decimal.
func f1(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }

// fv formats a whole-unit constant as build.py prints a float: "28.0".
func fv(x float64) string {
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// zeroSV slashes each 0 of an SVG label in Saira: Saira's 0 is the oval
// of its O, so the digit is set in Atkinson Hyperlegible Mono (VESPER.md
// 3.3). The text is escaped first.
func zeroSV(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "0", `<tspan class="gpw-sv-z">0</tspan>`)
}

const b0Plate = `<path class="gpw-sv-b0" d="M-34 -6H6V15L0 21H-34Z"/><circle class="gpw-sv-b0h" cx="0" cy="0" r="2.2"/>` +
	`<text class="gpw-sv-t-b0" x="-15" y="13.5" text-anchor="middle">B<tspan class="gpw-sv-z">0</tspan></text>`

// ringParts returns the parts of the drawing in paint order.
func ringParts(st ringState) []ringPart {
	rx, ry, r := ringC.x, ringC.y, ringR
	var parts []ringPart

	// The lanyard, from the top of the drawing through the dial's gap.
	lan := []pt{{rx - 5, 0}, {rx + 5, 0}, {rx + 5, ry - r - 10}, {rx + 1.5, ry - r + 1}, {rx - 1.5, ry - r + 1}, {rx - 5, ry - r - 10}}
	parts = append(parts, ringPart{"lanyard", fmt.Sprintf(`<path class="gpw-sv-lan" d="M%s 0V%sL%s %sH%sL%s %sV0Z"/>`,
		fv(rx-5), fv(ry-r-10), fv(rx-1.5), fv(ry-r+1), fv(rx+1.5), fv(rx+5), fv(ry-r-10)), [][]pt{lan}})

	// The Key behind the tags, its bow threaded on the ring at 4 o'clock,
	// and B0 on its own cord from the ring, clear to the left.
	if st.KeyOnRing {
		k := polar(keyAt, r, ringC)
		body, bow := keyOutline()
		parts = append(parts, ringPart{"key",
			fmt.Sprintf(`<g transform="translate(%s %s) rotate(%s) scale(%s) translate(0 5)"><path class="gpw-sv-keyf" fill-rule="evenodd" d="%s"/></g>`,
				f1(k.x), f1(k.y), fv(keyTurn), fv(keyS), keyPath),
			[][]pt{xf(body, k.x, k.y, keyTurn, keyS), xf(bow, k.x, k.y, keyTurn, keyS)}})
		c0 := polar(b0At, r+1, ringC)
		b := pt{c0.x + b0Cord.x, c0.y + b0Cord.y}
		plate := []pt{{-34, -6}, {6, -6}, {6, 15}, {0, 21}, {-34, 21}}
		parts = append(parts, ringPart{"b0",
			fmt.Sprintf(`<path class="gpw-sv-cord" d="M%s %sL%s %s"/><g transform="translate(%s %s) rotate(-5)">%s</g>`,
				f1(c0.x), f1(c0.y), f1(b.x), f1(b.y), f1(b.x), f1(b.y), b0Plate),
			[][]pt{xf(plate, b.x, b.y, -5, 1), segPoly(c0, b, 1.5)}})
	}

	// The steel ring with its centre lines.
	parts = append(parts, ringPart{"ring",
		fmt.Sprintf(`<path class="gpw-sv-ctr" d="M%s %sV%sM%s %sH%s"/><circle class="gpw-sv-steel" cx="%s" cy="%s" r="%s"/>`,
			fv(rx), fv(ry-r-12), fv(ry+r+10), fv(rx-r-12), fv(ry), fv(rx+r+12), fv(rx), fv(ry), fv(r)),
		[][]pt{circlePoly(rx, ry, r+2.6, 48)}})

	if st.Stamped > 0 && st.Front != nil {
		parts = append(parts, tagsPart(st))
	}
	if !st.KeyOnRing {
		parts = append(parts, hookPart(st.hook()))
	}
	if st.Open != nil && st.Open.N >= 1 && st.Open.N <= ringTiers {
		g := ghostAt[st.Open.N]
		parts = append(parts, ghostPart(st.Open, st.ToGo, float64(g.angle), float64(g.radius)))
	}
	return parts
}

// tagsPart is the stamped tags: the most recent in front, the edges of up
// to 2 earlier ones behind it, and +N for the earlier ones.
func tagsPart(st ringState) ringPart {
	t := polar(tagAt, ringR, ringC)
	h := tagHeight(st.Front)
	outline := fmt.Sprintf("M%s %sH%sV%sL%s %sH%sZ", fv(-tagW/2), fv(tagTop), fv(tagW/2), fv(h-10), fv(tagW/2-10), fv(h), fv(-tagW/2))
	var b strings.Builder
	var fp [][]pt
	for j := min(st.Stamped-1, 2); j > 0; j-- {
		dx, dy := t.x-stackAt[j], t.y+stackAt[j]
		fmt.Fprintf(&b, `<path class="gpw-sv-plate gpw-sv-stack" transform="translate(%s %s)" d="%s"/>`, f1(dx), f1(dy), outline)
		fp = append(fp, xf(tagOutline(h), dx, dy, 0, 1))
	}
	fmt.Fprintf(&b, `<g transform="translate(%s %s)"><path class="gpw-sv-halo" d="%s"/><path class="gpw-sv-plate" d="%s"/><circle class="gpw-sv-hole" cx="0" cy="0" r="4"/>`,
		f1(t.x), f1(t.y), outline, outline)
	fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-t-tag gpw-sv-t-plate" x="0" y="27" text-anchor="middle">%s</text><path class="gpw-sv-plate-rule" d="M-18 33H18"/>`,
		zeroSV(st.Front.Tag()))
	if st.Stamped > 1 {
		fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-t-stamp gpw-sv-t-plate" x="%s" y="7">+%s</text>`, fv(-tagW/2+4), zeroSV(itoa(st.Stamped-1)))
	}
	lines := st.Front.StampLines()
	for k, line := range lines {
		// A long name of 1 word is set 50 % wide, and fitted to the 48
		// units of the plate when even that is too wide (CONTAINMENT).
		cls, fit := "", ""
		if n := len([]rune(line)); len(lines) == 1 && n > 8 {
			cls = " gpw-sv-t-stamp-n"
			if n > 10 {
				fit = ` textLength="48" lengthAdjust="spacingAndGlyphs"`
			}
		}
		fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-t-stamp%s gpw-sv-t-plate" x="0" y="%d" text-anchor="middle"%s>%s</text>`, cls, 49+14*k, fit, zeroSV(line))
	}
	b.WriteString(`</g>`)
	fp = append(fp, xf(tagOutline(h), t.x, t.y, 0, 1))
	// The ring's wire through the front tag's hole: the tag is threaded
	// on it, so the wire shows over the plate for 18 degrees.
	w0, w1 := polar(tagAt-9, ringR, ringC), polar(tagAt+9, ringR, ringC)
	fmt.Fprintf(&b, `<path class="gpw-sv-steel" d="M%s %sA%s %s 0 0 1 %s %s"/>`, f1(w0.x), f1(w0.y), fv(ringR), fv(ringR), f1(w1.x), f1(w1.y))
	return ringPart{"tags", b.String(), fp}
}

// hookPart is the Key on its hook inside the dial, with its B0 tag on a
// cord and "ON ITS HOOK" in dial labels.
func hookPart(h hookPlace) ringPart {
	kx, ky := h.x, h.y
	x, y, tx, ty := float64(kx), float64(ky), float64(h.tx), float64(h.ty)
	body, bow := keyOutline()
	fp := [][]pt{
		xf(body, x, y, 0, 1.6), xf(bow, x, y, 0, 1.6),
		{{x - 40, y - 14}, {x + 4, y - 14}, {x + 4, y + 8}, {x - 40, y + 8}},
		xf([]pt{{-34, -6}, {6, -6}, {6, 21}, {-34, 21}}, x+float64(h.bx), y+float64(h.by), float64(h.brot), 1),
		// The label's 2 lines as set in Saira 600 at 15 units: "ON ITS" 53
		// wide, "HOOK" 44.
		{{tx - 2, ty - 14}, {tx + 55, ty - 14}, {tx + 55, ty + 4}, {tx - 2, ty + 4}},
		{{tx - 2, ty + 4}, {tx + 46, ty + 4}, {tx + 46, ty + 22}, {tx - 2, ty + 22}},
	}
	svg := fmt.Sprintf(`<path class="gpw-sv-cord" d="M%d %dL%d %d"/>`, kx-14, ky+12, kx+h.bx, ky+h.by) +
		fmt.Sprintf(`<g transform="translate(%d %d) scale(1.6) translate(0 5)"><path class="gpw-sv-keyf" fill-rule="evenodd" d="%s"/></g>`, kx, ky, keyPath) +
		fmt.Sprintf(`<path class="gpw-sv-h2" d="M%d %dh7v22h-7Z"/><path class="gpw-sv-hook" d="M%d %dH%dV%d"/>`, kx-40, ky-14, kx-33, ky+2, kx+4, ky-6) +
		fmt.Sprintf(`<g transform="translate(%d %d) rotate(%d)">%s</g>`, kx+h.bx, ky+h.by, h.brot, b0Plate) +
		fmt.Sprintf(`<text class="gpw-sv-t gpw-sv-t-dial" x="%d" y="%d">ON ITS</text><text class="gpw-sv-t gpw-sv-t-dial" x="%d" y="%d">HOOK</text>`, h.tx, h.ty, h.tx, h.ty+18)
	return ringPart{"hook", svg, fp}
}

// ghostPart is the open tier's dashed tag inside its own dial segment, on
// a short leader from the segment's inner edge.
func ghostPart(t *course.Tier, toGo int, a, rg float64) ringPart {
	g := polar(a, rg, dialC)
	g.y -= (ghostTop + ghostH) / 2
	poly := xf(ghostOutline(), g.x, g.y, 0, 1)
	s := polar(a, dialR-8, dialC)
	// The leader meets the nearest point of the ghost's top or bottom
	// edge, or its side in a segment at 3 or 9 o'clock.
	l := pt{min(max(s.x, g.x-ghostW/2+4), g.x+ghostW/2-4), g.y + ghostH - 2}
	if s.y < g.y {
		l.y = g.y + ghostTop
	}
	if math.Abs(s.y-g.y) < ghostH/2+4 && math.Abs(s.x-g.x) > ghostW/2 {
		l.x = g.x - ghostW/2
		if s.x > g.x {
			l.x = g.x + ghostW/2
		}
		l.y = min(max(s.y, g.y), g.y+ghostH-12)
	}
	w2 := ghostW / 2
	svg := fmt.Sprintf(`<path class="gpw-sv-am gpw-sv-dash" d="M%s %sL%s %s"/>`, f1(s.x), f1(s.y), f1(l.x), f1(l.y)) +
		fmt.Sprintf(`<g transform="translate(%s %s)"><path class="gpw-sv-am gpw-sv-dash gpw-sv-ghost" d="M%s %sH%sV%sL%s %sH%sZ"/>`,
			f1(g.x), f1(g.y), fv(-w2), fv(ghostTop), fv(w2), fv(ghostH-8), fv(w2-8), fv(ghostH), fv(-w2)) +
		`<circle class="gpw-sv-hole" cx="0" cy="0" r="3"/>` +
		fmt.Sprintf(`<text class="gpw-sv-t gpw-sv-t-gh gpw-sv-t-am" x="0" y="19.5" text-anchor="middle">%s</text>`, zeroSV(t.Tag())) +
		`<path class="gpw-sv-h2" d="M-13 23.5H13"/>` +
		fmt.Sprintf(`<text class="gpw-sv-t gpw-sv-t-gc gpw-sv-t-q" x="0" y="34" text-anchor="middle">%s TO GO</text></g>`, zeroSV(itoa(toGo)))
	return ringPart{"ghost", svg, [][]pt{poly, segPoly(s, l, 1.5)}}
}

// dialArc is an arc of the dial from angle a0 to a1 at radius r.
func dialArc(a0, a1, r float64) string {
	p0, p1 := polar(a0, r, dialC), polar(a1, r, dialC)
	return fmt.Sprintf("M%s %sA%s %s 0 0 1 %s %s", f1(p0.x), f1(p0.y), fv(r), fv(r), f1(p1.x), f1(p1.y))
}

// dialBand is a band of the dial from radius 132 to 140.
func dialBand(cls string, a0, a1 float64) string {
	p1, p0 := polar(a1, dialR-4, dialC), polar(a0, dialR-4, dialC)
	return fmt.Sprintf(`<path class="%s" d="%sL%s %sA%s %s 0 0 0 %s %sZ"/>`,
		cls, dialArc(a0, a1, dialR+4), f1(p1.x), f1(p1.y), fv(dialR-4), fv(dialR-4), f1(p0.x), f1(p0.y))
}

// ringSVG draws the ring: the tier dial, then the parts. id names the
// <title>, the drawing's text alternative.
func ringSVG(st ringState, id, title string) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="gpw-ring-draw" viewBox="0 0 340 340" role="img" aria-labelledby="%s"><title id="%s">%s</title>`,
		html.EscapeString(id), html.EscapeString(id), html.EscapeString(title))
	for i := range ringTiers {
		n := i + 1
		a0 := float64(-90 + i*36 + 3)
		a1 := a0 + 30
		open := st.Open != nil && st.Open.N == n
		switch {
		case st.Bands[n] == bandStamped:
			b.WriteString(dialBand("gpw-sv-amf", a0, a1))
		case open:
			fmt.Fprintf(&b, `<path class="gpw-sv-am gpw-sv-dash" d="%s"/>`, dialArc(a0, a1, dialR))
			if total := st.Done + st.ToGo; st.Done > 0 && total > 0 {
				b.WriteString(dialBand("gpw-sv-amf", a0, a0+(a1-a0)*float64(st.Done)/float64(total)))
			}
		case st.Bands[n] == bandReady:
			// No frame draws it (VESPER.md 16.1): the full band as a 2 px
			// amber keyline with no fill. Solid amber means stamped only.
			b.WriteString(dialBand("gpw-sv-ready", a0, a1))
		default:
			fmt.Fprintf(&b, `<path class="gpw-sv-h2" d="%s"/>`, dialArc(a0, a1, dialR))
		}
		for _, a := range []float64{a0, a1} {
			p0, p1 := polar(a, dialR-6, dialC), polar(a, dialR+6, dialC)
			fmt.Fprintf(&b, `<path class="gpw-sv-h" d="M%s %sL%s %s"/>`, f1(p0.x), f1(p0.y), f1(p1.x), f1(p1.y))
		}
		l := polar(a0+15, dialR+20, dialC)
		am := ""
		if st.Bands[n] == bandStamped || open {
			am = " gpw-sv-t-am"
		}
		fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-t-dial%s" x="%s" y="%s" text-anchor="middle">%s</text>`, am, f1(l.x), f1(l.y+5), zeroSV(fmt.Sprintf("T%d", n)))
	}
	for _, p := range ringParts(st) {
		fmt.Fprintf(&b, `<g data-ring-part="%s">%s</g>`, p.name, p.svg)
	}
	b.WriteString(`</svg>`)
	// Every value in the markup is a number, a fixed string or escaped
	// above.
	return template.HTML(b.String()) //nolint:gosec // see above
}
