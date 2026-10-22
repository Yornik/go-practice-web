package cert

import (
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"rsc.io/qr"

	"github.com/yornik/go-practice-web/internal/course"
)

// Document is what a certificate PDF shows.
type Document struct {
	ID          string
	Kind        Kind
	Name        string
	IssuedAt    time.Time
	Title       string // "Tier 6, Forensics" or "The complete course"
	Tier        int    // the tier a tier certificate covers; 0 for the course or a tier that no longer exists
	TierName    string // "Forensics"; "" when the course names no tiers
	TierLine    string // what the sheet says after the colon; "" when the story has no line for the tier
	Range       string // the exercises it covers, "26 to 28"
	Bars        []Bar  // the course to scale, for the course certificate
	Items       []Item // the tier's exercises
	Count       int    // the number of exercises covered
	VerifyURL   string // https://vesper-labs.org/verify/GP-...
	Host        string // the portal's address in the issuer line, "vesper-labs.org"
	Fingerprint string
	Stats       *Stats // nil hides the activity
	Revoked     bool   // stamps the page as revoked
	ReplacedBy  string // the certificate that replaced a revoked one; may be ""
	IssuedAs    string // what a certificate of the first course was issued as (Coverage.IssuedAs); "" otherwise
}

// Bar is 1 exercise of the course's skyline: as tall as its hours.
type Bar struct {
	Tier  int
	Hours float64
}

// Item is a line of the list of exercises.
type Item struct {
	Number string // "04"
	Title  string
}

// NewDocument builds the PDF content from a verified payload, so the PDF
// shows exactly what was signed, read through the course's renames (see
// Interpret). siteURL is GPW_PUBLIC_URL.
func NewDocument(c *course.Course, p Payload, siteURL, fingerprint string) (Document, error) {
	issued, err := p.Issued()
	if err != nil {
		return Document{}, err
	}
	if p.Kind != KindCourse {
		if _, ok := p.Kind.Tier(); !ok {
			return Document{}, fmt.Errorf("%w: %q", ErrKind, p.Kind)
		}
	}
	cv := Interpret(c, p)
	site := strings.TrimSuffix(siteURL, "/")
	host := site
	if u, err := url.Parse(site); err == nil && u.Host != "" {
		host = u.Host
	}
	d := Document{
		ID:          p.ID,
		Kind:        p.Kind,
		Name:        p.Name,
		IssuedAt:    issued,
		Title:       cv.Title(),
		Range:       cv.Range(),
		VerifyURL:   site + "/verify/" + p.ID,
		Host:        host,
		Fingerprint: fingerprint,
		Stats:       p.Stats,
		Count:       len(p.Exercises),
		IssuedAs:    cv.IssuedAs(),
	}
	if p.Kind == KindCourse {
		for _, e := range cv.Exercises {
			d.Bars = append(d.Bars, Bar{Tier: e.Tier, Hours: e.Hours})
		}
		return d, nil
	}
	if cv.Tier != nil {
		d.Tier, d.TierName, d.TierLine = cv.Tier.N, cv.Tier.Name, cv.Tier.Line()
	}
	for _, e := range cv.Exercises {
		d.Items = append(d.Items, Item{Number: fmt.Sprintf("%02d", e.Number), Title: e.Title})
	}
	return d, nil
}

// duration writes active time as the site does: "35 min", "2 h",
// "21 h 40 min".
func duration(sec int64) string {
	m := sec / 60
	switch {
	case m < 60:
		return fmt.Sprintf("%d min", m)
	case m%60 == 0:
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// The sheet (VESPER.md 13.1): A4 landscape in points. A frame 10 mm in,
// lettered A to L like the site's 12-column ruler; the head; the art in
// zones A to C beside the text in zones D to L; the foot with the issuer
// line, the QR code and the title block.
const (
	pageW = 841.89
	pageH = 595.28
	mm    = 72 / 25.4

	frameL = 10 * mm
	frameT = 10 * mm
	frameR = pageW - 10*mm
	frameB = pageH - 10*mm
	zoneW  = (frameR - frameL) / 12
	pad    = 6 * mm  // the text's inset from the frame
	cut    = 8 * mm  // the frame's chamfer, bottom right
	band   = 12 * mm // the revoked band
	headH  = 22 * mm
	footH  = 44 * mm
	tbW    = 110 * mm // the title block
	tbRowH = 11 * mm
	tbMin  = 8.5 // the smallest value in the title block
	textL  = frameL + 3*zoneW
	textR  = frameR - pad
	artCX  = frameL + 1.5*zoneW + 2.5*mm
)

// centreDash is a centre line: long dash, dot.
var centreDash = []float64{6, 2, 1.2, 2}

type rgb struct{ r, g, b uint8 }

// The inks of the paper: black and 1 amber, so the sheet prints on any
// printer. Ink 2, the hairline and the rule are tints of the black. Amber
// is a fill only, never text, always under black ink; on a 1-colour
// printer it prints as a mid grey that black still reads on.
var (
	paper = rgb{0xFF, 0xFF, 0xFF}
	ink   = rgb{0x07, 0x07, 0x07}
	ink2  = rgb{0x54, 0x56, 0x59}
	hair  = rgb{0x76, 0x79, 0x7D}
	rule  = rgb{0xD6, 0xD7, 0xD9}
	amber = rgb{0xFF, 0xAA, 0x00}
)

// The mark at 32 units: the plate, the V and the hole cut out of it, and
// the star in the hole (the site's mark.svg).
var (
	markPlate = []gopdf.Point{{X: 0, Y: 0}, {X: 32, Y: 0}, {X: 32, Y: 25}, {X: 25, Y: 32}, {X: 0, Y: 32}}
	markV     = []gopdf.Point{{X: 4.5, Y: 12}, {X: 10.75, Y: 12}, {X: 16, Y: 22.5}, {X: 21.25, Y: 12}, {X: 27.5, Y: 12}, {X: 19.4, Y: 28}, {X: 12.6, Y: 28}}
	markHole  = []gopdf.Point{{X: 16, Y: 2.6}, {X: 19.4, Y: 6}, {X: 16, Y: 9.4}, {X: 12.6, Y: 6}}
	markStar  = []gopdf.Point{{X: 16, Y: 4.1}, {X: 17.9, Y: 6}, {X: 16, Y: 7.9}, {X: 14.1, Y: 6}}
)

// canvas wraps gopdf with a sticky error, so the layout code reads as a
// list of drawing steps. A dry canvas measures and draws nothing: the
// sheet measures its body that way before it centres it.
type canvas struct {
	pdf    *gopdf.GoPdf
	fonts  map[string]fontData
	dry    bool
	err    error
	onText func(string) // a test hook: every run of text drawn
}

func (c *canvas) fail(err error) {
	if c.err == nil && err != nil {
		c.err = err
	}
}

func (c *canvas) font(family string, size, spacing float64) {
	c.fail(c.pdf.SetFont(family, "", size))
	c.fail(c.pdf.SetCharSpacing(spacing))
}

// capH is the cap height of a family at a size.
func (c *canvas) capH(family string, size float64) float64 { return c.fonts[family].cap * size }

// width measures s in gopdf's own arithmetic. With spacing, gopdf adds
// the spacing after every letter, the last one too.
func (c *canvas) width(family string, size, spacing float64, s string) float64 {
	c.font(family, size, spacing)
	w, err := c.pdf.MeasureTextWidth(s)
	c.fail(err)
	return w
}

func (c *canvas) measure(family string, size float64, s string) float64 {
	return c.width(family, size, 0, s)
}

// text draws s with its baseline at y and returns its width.
func (c *canvas) text(x, y float64, s, family string, size float64, col rgb, spacing float64) float64 {
	w := c.width(family, size, spacing, s)
	if c.dry {
		return w
	}
	c.pdf.SetTextColor(col.r, col.g, col.b)
	c.pdf.SetXY(x, y)
	c.fail(c.pdf.Text(s))
	if c.onText != nil {
		c.onText(s)
	}
	return w
}

// right and centre take the last letter's spacing off, so the letters,
// not the space after them, line up.
func (c *canvas) right(x, y float64, s, family string, size float64, col rgb, spacing float64) {
	c.text(x-(c.width(family, size, spacing, s)-spacing), y, s, family, size, col, spacing)
}

func (c *canvas) centre(cx, y float64, s, family string, size float64, col rgb, spacing float64) {
	c.text(cx-(c.width(family, size, spacing, s)-spacing)/2, y, s, family, size, col, spacing)
}

// upright draws s turned a quarter to the left, centred on (x, cy): the
// label of a vertical dimension.
func (c *canvas) upright(x, cy float64, s, family string, size float64, col rgb) {
	y := cy + c.width(family, size, 0, s)/2
	if c.dry {
		return
	}
	c.pdf.Rotate(90, x, y)
	c.text(x, y, s, family, size, col, 0)
	c.pdf.RotateReset()
}

func (c *canvas) stroke(col rgb, w float64, dash []float64) {
	c.pdf.SetLineWidth(w)
	c.pdf.SetStrokeColor(col.r, col.g, col.b)
	if len(dash) > 0 {
		c.pdf.SetCustomLineType(dash, 0)
		return
	}
	c.pdf.SetLineType("solid")
}

func (c *canvas) line(x1, y1, x2, y2 float64, col rgb, w float64, dash ...float64) {
	if c.dry {
		return
	}
	c.stroke(col, w, dash)
	c.pdf.Line(x1, y1, x2, y2)
}

func (c *canvas) rect(x, y, w, h float64, col rgb) {
	if c.dry {
		return
	}
	c.pdf.SetFillColor(col.r, col.g, col.b)
	c.pdf.RectFromUpperLeftWithStyle(x, y, w, h, "F")
}

// shape fills a closed polygon, strokes it, or both. A nil fill or a
// zero stroke width leaves that part out.
func (c *canvas) shape(pts []gopdf.Point, fill *rgb, stroke rgb, sw float64) {
	if c.dry {
		return
	}
	style := ""
	if fill != nil {
		c.pdf.SetFillColor(fill.r, fill.g, fill.b)
		style = "F"
	}
	if sw > 0 {
		c.stroke(stroke, sw, nil)
		style = "D" + style
	}
	c.pdf.Polygon(pts, style)
}

// open strokes an open polyline. gopdf has no polyline and no line cap,
// so each segment runs on by half the line width at an inner corner: the
// corner is square, not notched.
func (c *canvas) open(pts []gopdf.Point, col rgb, w float64) {
	if c.dry {
		return
	}
	c.stroke(col, w, nil)
	for i := 1; i < len(pts); i++ {
		a, z := pts[i-1], pts[i]
		dx, dy := z.X-a.X, z.Y-a.Y
		l := math.Hypot(dx, dy)
		ex, ey := dx/l*w/2, dy/l*w/2
		if i > 1 {
			a.X, a.Y = a.X-ex, a.Y-ey
		}
		if i < len(pts)-1 {
			z.X, z.Y = z.X+ex, z.Y+ey
		}
		c.pdf.Line(a.X, a.Y, z.X, z.Y)
	}
}

// circle fills and strokes a circle. gopdf's Oval only strokes, so a
// filled circle is a polygon of 96 points, 0.02 pt off a true circle.
func (c *canvas) circle(cx, cy, r float64, fill, stroke rgb, sw float64) {
	pts := make([]gopdf.Point, 96)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / 96
		pts[i] = gopdf.Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)}
	}
	c.shape(pts, &fill, stroke, sw)
}

// fit is the largest size from hi down to lo, in steps of 0.5 pt, at
// which s fits in maxW; lo when none does.
func (c *canvas) fit(family string, hi, lo float64, s string, maxW, spacing float64) float64 {
	for size := hi; size > lo; size -= 0.5 {
		if c.width(family, size, spacing, s) <= maxW {
			return size
		}
	}
	return lo
}

// wrap breaks s at spaces into lines no wider than maxW.
func (c *canvas) wrap(family string, size float64, s string, maxW float64) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Split(s, " ") {
		t := word
		if cur != "" {
			t = cur + " " + word
		}
		if cur != "" && c.measure(family, size, t) > maxW {
			lines = append(lines, cur)
			cur = word
			continue
		}
		cur = t
	}
	return append(lines, cur)
}

// span is a piece of a paragraph in 1 font. glue puts a space before it.
type span struct {
	text   string
	family string
	size   float64
	col    rgb
	glue   bool
}

// rich draws 1 paragraph of mixed fonts with its first baseline at y,
// wrapped at spaces in maxW, on rows of lead. A mono span never breaks:
// it is a command. Words of 1 font on 1 line become 1 text run. It
// returns the baseline of the last line.
func (c *canvas) rich(x, y float64, spans []span, maxW, lead float64) float64 {
	var words []span
	for _, s := range spans {
		parts := strings.Split(s.text, " ")
		if s.family == fontMono {
			parts = []string{s.text}
		}
		for i, p := range parts {
			w := s
			w.text, w.glue = p, s.glue || i > 0
			words = append(words, w)
		}
	}
	runs := func(ws []span) []span {
		var out []span
		for _, w := range ws {
			if n := len(out); n > 0 && out[n-1].family == w.family && out[n-1].size == w.size && out[n-1].col == w.col {
				if w.glue {
					out[n-1].text += " "
				}
				out[n-1].text += w.text
				continue
			}
			if n := len(out); n > 0 && w.glue {
				out[n-1].text += " "
			}
			out = append(out, w)
		}
		return out
	}
	width := func(ws []span) float64 {
		var t float64
		for _, r := range runs(ws) {
			t += c.measure(r.family, r.size, r.text)
		}
		return t
	}
	var lines [][]span
	var cur []span
	for _, w := range words {
		if len(cur) > 0 && w.glue && width(append(append([]span(nil), cur...), w)) > maxW {
			lines = append(lines, cur)
			w.glue = false
			cur = []span{w}
			continue
		}
		cur = append(cur, w)
	}
	lines = append(lines, cur)
	for i, l := range lines {
		cx := x
		for _, r := range runs(l) {
			cx += c.text(cx, y, r.text, r.family, r.size, r.col, 0)
		}
		if i < len(lines)-1 {
			y += lead
		}
	}
	return y
}

// Render writes the certificate as a 1-page PDF.
func Render(w io.Writer, d Document) error {
	pdf, fonts, err := newPDF()
	if err != nil {
		return err
	}
	pdf.SetInfo(gopdf.PdfInfo{
		Title:        "Go Practice certificate " + d.ID,
		Author:       "Go Practice",
		Subject:      d.Title + " passed by " + d.Name,
		Creator:      "Go Practice",
		Producer:     "go-practice-web",
		CreationDate: d.IssuedAt,
	})
	pdf.AddPage()
	c := &canvas{pdf: pdf, fonts: fonts}
	if _, err := drawSheet(c, d); err != nil {
		return fmt.Errorf("cert: render %s: %w", d.ID, err)
	}
	if _, err := pdf.WriteTo(w); err != nil {
		return fmt.Errorf("cert: write %s: %w", d.ID, err)
	}
	return nil
}

// sheetLayout is where the body landed, for the tests: the body's box and
// the bottom of what the art and the text drew in it.
type sheetLayout struct {
	bodyTop, bodyBottom, contentBottom float64
}

// drawSheet draws the whole page.
func drawSheet(c *canvas, d Document) (sheetLayout, error) {
	c.rect(0, 0, pageW, pageH, paper)
	drawFrame(c)
	top := frameT
	if d.Revoked {
		drawRevoked(c, d.ReplacedBy)
		top += band
	}
	bodyTop := drawHead(c, d, top)
	bodyBottom := frameB - footH
	art, text := drawTag, drawTierText
	if d.Kind == KindCourse {
		art, text = drawKey, drawCourseText
	}
	// The art and the text start on 1 line, and the pair sits in the
	// middle of the body: a dry run measures it first.
	dry := &canvas{pdf: c.pdf, fonts: c.fonts, dry: true}
	h := max(art(dry, d, 0), text(dry, d, 0))
	c.fail(dry.err)
	topC := bodyTop + max(8*mm, (bodyBottom-bodyTop-h)/2)
	bottom := max(art(c, d, topC), text(c, d, topC))
	drawFoot(c, d)
	return sheetLayout{bodyTop, bodyBottom, bottom}, c.err
}

// drawFrame draws the zone ticks and letters above the frame, and the
// frame with its chamfer.
func drawFrame(c *canvas) {
	for i := range 13 {
		x := frameL + float64(i)*zoneW
		c.line(x, frameT, x, frameT-2.5*mm, hair, 0.5)
	}
	for i, z := range "ABCDEFGHIJKL" {
		c.centre(frameL+(float64(i)+0.5)*zoneW, frameT-2.4*mm, string(z), fontKind, 8.5, ink2, 0)
	}
	c.shape([]gopdf.Point{{X: frameL, Y: frameT}, {X: frameR, Y: frameT}, {X: frameR, Y: frameB - cut}, {X: frameR - cut, Y: frameB}, {X: frameL, Y: frameB}}, nil, ink, 0.75)
}

// drawRevoked puts a black band inside the top of the frame. Black, not
// red: the sheet keeps its 2 inks, and the word carries the meaning.
func drawRevoked(c *canvas, replacedBy string) {
	c.rect(frameL, frameT, frameR-frameL, band, ink)
	w := c.text(frameL+pad, frameT+band/2+c.capH(fontName, 14)/2, "REVOKED", fontName, 14, paper, 1.4)
	msg := "This certificate is no longer valid."
	if replacedBy != "" {
		msg = "This certificate was replaced by " + replacedBy + "."
	}
	c.text(frameL+pad+w+5*mm, frameT+band/2+c.capH(fontBold, 11)/2, msg, fontBold, 11, paper, 0)
}

// drawHead draws the mark, the product and the course, and the kind line
// in the building's voice, and returns the y of its closing rule.
func drawHead(c *canvas, d Document, top float64) float64 {
	mx, my, sc := frameL+pad, top+6*mm, 10*mm/32
	at := func(pts []gopdf.Point) []gopdf.Point {
		out := make([]gopdf.Point, len(pts))
		for i, p := range pts {
			out[i] = gopdf.Point{X: mx + p.X*sc, Y: my + p.Y*sc}
		}
		return out
	}
	// gopdf fills 1 path at a time, so the cut-outs are paper drawn over
	// the plate. A revoked sheet carries no amber: the star stays empty.
	c.shape(at(markPlate), &ink, ink, 0)
	c.shape(at(markV), &paper, ink, 0)
	c.shape(at(markHole), &paper, ink, 0)
	if !d.Revoked {
		c.shape(at(markStar), &amber, ink, 0)
	}
	tx := mx + 10*mm + 3.5*mm
	c.text(tx, my+c.capH(fontBold, 14), "Go Practice", fontBold, 14, ink, 0)
	c.text(tx, my+10*mm, "The go-practice course", fontText, 9.5, ink2, 0)
	kind := "TIER CERTIFICATE"
	if d.Kind == KindCourse {
		kind = "COURSE CERTIFICATE"
	}
	c.right(textR, my+10*mm, kind, fontKind, 24, ink, 2.4)
	y := top + headH
	c.line(frameL, y, frameR, y, ink, 1.5)
	return y
}

// brackets draws open corner brackets round a box and the ghost bracket 1
// layer behind it, and returns the bottom of the ghost.
func brackets(c *canvas, x, y, w, h float64) float64 {
	const o, a, g = 3 * mm, 5 * mm, 3.2 * mm
	x0, y0, x1, y1 := x-o, y-o, x+w+o, y+h+o
	for _, k := range [][4]float64{{x0, y0, 1, 1}, {x1, y0, -1, 1}, {x0, y1, 1, -1}, {x1, y1, -1, -1}} {
		cx, cy, sx, sy := k[0], k[1], k[2], k[3]
		c.open([]gopdf.Point{{X: cx, Y: cy + sy*a}, {X: cx, Y: cy}, {X: cx + sx*a, Y: cy}}, ink, 0.6)
	}
	for _, k := range [][4]float64{{x1, y0, -1, 1}, {x0, y1, 1, -1}, {x1, y1, -1, -1}} {
		cx, cy, sx, sy := k[0]+g, k[1]+g, k[2], k[3]
		c.open([]gopdf.Point{{X: cx, Y: cy + sy*a}, {X: cx, Y: cy}, {X: cx + sx*a, Y: cy}}, hair, 0.5)
	}
	return y1 + g
}

// dimH and dimV draw a dimension: extension lines, the line with 45
// degree ticks, the size beside it.
func dimH(c *canvas, x0, x1, y float64, label string) {
	for _, x := range []float64{x0, x1} {
		c.line(x, y-2*mm, x, y+1.5*mm, hair, 0.4)
		c.line(x-0.9*mm, y+0.9*mm, x+0.9*mm, y-0.9*mm, ink2, 0.6)
	}
	c.line(x0, y, x1, y, ink2, 0.5)
	c.centre((x0+x1)/2, y-1.4*mm, label, fontKind, 8.5, ink2, 0)
}

func dimV(c *canvas, x, y0, y1 float64, label string) {
	for _, y := range []float64{y0, y1} {
		c.line(x-1.5*mm, y, x+2*mm, y, hair, 0.4)
		c.line(x-0.9*mm, y+0.9*mm, x+0.9*mm, y-0.9*mm, ink2, 0.6)
	}
	c.line(x, y0, x, y1, ink2, 0.5)
	c.upright(x-1.4*mm, (y0+y1)/2, label, fontKind, 8.5, ink2)
}

// detailLabel is the drawing's caption above the art.
func detailLabel(c *canvas, top float64, s string) {
	c.text(frameL+pad, top+c.capH(fontName, 8.5), s, fontName, 8.5, ink2, 0.85)
}

// drawTag is Detail A: the tier's tag at 1:1, a chamfered steel plate with
// a pressed rim and a punched hole, stamped. Stamped means amber, as on
// the ring; a revoked sheet carries no amber. It returns its bottom.
func drawTag(c *canvas, d Document, top float64) float64 {
	id := fmt.Sprintf("T%d", tierNumber(d))
	detailLabel(c, top, "DETAIL A: TAG "+id+", SCALE 1:1")
	const pw, ph, ch, r = 44 * mm, 58 * mm, 7 * mm, 1.6 * mm
	px, py := artCX-pw/2, top+9*mm
	face := amber
	if d.Revoked {
		face = paper
	}
	c.shape([]gopdf.Point{{X: px, Y: py}, {X: px + pw, Y: py}, {X: px + pw, Y: py + ph - ch}, {X: px + pw - ch, Y: py + ph}, {X: px, Y: py + ph}}, &face, ink, 0.75)
	k := r - r*math.Sqrt2
	c.shape([]gopdf.Point{{X: px + r, Y: py + r}, {X: px + pw - r, Y: py + r}, {X: px + pw - r, Y: py + ph - ch + k}, {X: px + pw - ch + k, Y: py + ph - r}, {X: px + r, Y: py + ph - r}}, nil, ink, 0.4)
	hx, hy := artCX, py+6.5*mm
	c.circle(hx, hy, 2*mm, paper, ink, 0.75)
	c.centre(artCX, py+35*mm, id, fontName, c.fit(fontName, 76, 44, id, pw-12*mm, 0), ink, 0)
	if d.TierName != "" {
		stamp := strings.ToUpper(d.TierName)
		c.centre(artCX, py+43*mm, stamp, fontName, c.fit(fontName, 13, 9, stamp, pw-9*mm, 1.3), ink, 1.3)
	}
	c.line(px+10*mm, py+46.5*mm, px+pw-10*mm, py+46.5*mm, ink, 0.5)
	if len(d.Items) > 0 {
		span := d.Items[0].Number + " TO " + d.Items[len(d.Items)-1].Number
		c.centre(artCX, py+52*mm, span, fontKind, 10, ink, 1)
	}
	c.line(hx, py-5*mm, hx, py+12*mm, ink2, 0.4, centreDash...)
	c.line(hx-7.5*mm, hy, hx+7.5*mm, hy, ink2, 0.4, centreDash...)
	bottom := brackets(c, px, py, pw, ph)
	dimH(c, px, px+pw, bottom+5*mm, "44")
	dimV(c, px-9*mm, py, py+ph, "58")
	return bottom + 6*mm
}

// drawKey is Detail B: the Key at 1:1 (VESPER.md 6.28) on its tag,
// Breaker 0, Hall 3. Amber on a current sheet, paper on a revoked one.
// gopdf fills 1 path at a time, so the order matters: the bow, the
// shaft's fill over the bow's edge, the shaft's outline, the hole.
func drawKey(c *canvas, d Document, top float64) float64 {
	detailLabel(c, top, "DETAIL B: THE KEY, SCALE 1:1")
	face := amber
	if d.Revoked {
		face = paper
	}
	const r0, r1, hw = 12 * mm, 5 * mm, 3 * mm
	bx, by := artCX+4*mm, top+23*mm
	js := by + math.Sqrt(r0*r0-hw*hw) // where the shaft leaves the bow
	p := func(x, y float64) gopdf.Point { return gopdf.Point{X: bx + x, Y: by + y} }
	body := []gopdf.Point{p(hw, 36*mm), p(11*mm, 36*mm), p(11*mm, 41*mm), p(hw, 41*mm), p(hw, 44*mm), p(8*mm, 44*mm),
		p(8*mm, 48.5*mm), p(hw, 48.5*mm), p(hw, 52*mm), p(11*mm, 52*mm), p(11*mm, 60*mm), p(-hw, 60*mm)}
	c.circle(bx, by, r0, face, ink, 0.75)
	c.shape(append([]gopdf.Point{p(-hw, 8*mm), p(hw, 8*mm)}, body...), &face, ink, 0)
	c.open(append(append([]gopdf.Point{{X: bx + hw, Y: js}}, body...), gopdf.Point{X: bx - hw, Y: js}), ink, 0.75)
	c.circle(bx, by, r1, paper, ink, 0.75)

	// The tag on its cord, from the bow's edge to the tag's hole.
	const tw, th, tc = 17 * mm, 12 * mm, 2.5 * mm
	hx, hy := bx-r0-6*mm, by+7*mm
	tx, ty := hx+3*mm-tw, hy-3*mm
	ux, uy := hx-bx, hy-by
	ul := math.Hypot(ux, uy)
	ux, uy = ux/ul, uy/ul
	c.shape([]gopdf.Point{{X: tx, Y: ty}, {X: tx + tw, Y: ty}, {X: tx + tw, Y: ty + th - tc}, {X: tx + tw - tc, Y: ty + th}, {X: tx, Y: ty + th}}, &paper, ink, 0.75)
	c.circle(hx, hy, 1.1*mm, paper, ink, 0.6)
	c.line(bx+ux*r0, by+uy*r0, hx-ux*1.1*mm, hy-uy*1.1*mm, ink, 0.9)
	c.centre(tx+6.6*mm, ty+6*mm, "B0", fontKind, 13, ink, 0)
	c.centre(tx+6.6*mm, ty+9.8*mm, "HALL 3", fontName, 8.5, ink2, 0.85)

	c.line(bx, by-r0-4*mm, bx, by+64*mm, ink2, 0.4, centreDash...)
	c.line(bx-r0-3*mm, by, bx+r0+3*mm, by, ink2, 0.4, centreDash...)
	dimV(c, bx+16*mm, by-r0, by+60*mm, "72")
	return by + 64*mm
}

// drawName sets the name at the top of the text column and returns the
// baseline of its last line.
func drawName(c *canvas, d Document, top float64) float64 {
	l := layoutName(c.measure, nameFont(c.pdf, d.Name), d.Name, textR-textL)
	y := top + c.capH(l.family, l.size)
	for i, s := range l.lines {
		c.text(textL, y, s, l.family, l.size, ink, 0)
		if i < len(l.lines)-1 {
			y += l.size * nameLead
		}
	}
	return y
}

// lead draws the line under the name: Atkinson 15 pt on 21 pt rows,
// 160 mm wide, its first baseline 13 mm under the name's.
func lead(c *canvas, y float64, s string) float64 {
	y += 13 * mm
	for _, l := range c.wrap(fontText, 15, s, 160*mm) {
		c.text(textL, y, l, fontText, 15, ink, 0)
		y += 21
	}
	return y
}

// checked is the sentence that says what the site checked, with the
// command in mono, and, for a certificate of the first course, the line
// that says what it was issued as.
func checked(c *canvas, d Document, y float64, what string) float64 {
	y = c.rich(textL, y+1, []span{
		{what + " passed its full test suite with", fontText, 11, ink2, true},
		{"go test -race", fontMono, 10.5, ink, true},
		{".", fontText, 11, ink2, false},
	}, textR-textL, 16)
	if d.IssuedAs != "" {
		y += 2
		for _, l := range c.wrap(fontText, 9.5, d.IssuedAs, textR-textL) {
			y += 13
			c.text(textL, y, l, fontText, 9.5, ink2, 0)
		}
	}
	return y
}

// tierNumber is the tier the sheet names: the tier that holds the
// exercises, or, when they span tiers, the number in the kind.
func tierNumber(d Document) int {
	if d.Tier > 0 {
		return d.Tier
	}
	n, _ := d.Kind.Tier()
	return n
}

// drawTierText is the text of a tier certificate: the name, the line, the
// check, the exercises and the activity. It returns its bottom.
func drawTierText(c *canvas, d Document, top float64) float64 {
	y := drawName(c, d, top)
	what := fmt.Sprintf("Tier %d", tierNumber(d))
	if d.TierName != "" {
		what += ", " + d.TierName + ","
	}
	line := "completed " + what + " of the go-practice course."
	if d.TierLine != "" {
		line = "completed " + what + " of the go-practice course: " + d.TierLine + "."
	}
	y = lead(c, y, line)
	each := fmt.Sprintf("Each of the %d exercises", d.Count)
	if d.Count == 1 {
		each = "The exercise"
	}
	y = checked(c, d, y, each)

	y += 8 * mm
	c.line(textL, y, textR, y, ink, 0.75)
	cols := 2
	if len(d.Items) <= 3 {
		cols = 1
	}
	rows := (len(d.Items) + cols - 1) / cols
	const gap, rowH = 8 * mm, 21.0
	colW := (textR - textL) / 2
	if cols == 2 {
		colW = (textR - textL - gap) / 2
	}
	for i, it := range d.Items {
		x := textL + float64(i/rows)*(colW+gap)
		ry := y + float64(i%rows)*rowH
		c.text(x, ry+14.5, it.Number, fontKind, 11, ink2, 0)
		c.text(x+9*mm, ry+14.5, it.Title, fontText, c.fit(fontText, 11.5, 8.5, it.Title, colW-9*mm, 0), ink, 0)
		c.line(x, ry+rowH, x+colW, ry+rowH, rule, 0.5)
	}
	return drawActivity(c, d, y+float64(rows)*rowH)
}

// skyMax is the hours of the tallest tower: 18 mm stand for 6 h.
const skyMax = 6.0

// drawCourseText is the text of the course certificate: the name, the
// line, the check, the course to scale and the activity.
func drawCourseText(c *canvas, d Document, top float64) float64 {
	y := drawName(c, d, top)
	y = lead(c, y, fmt.Sprintf("held the Key through %s and wrote every tool by hand. Every pager in Meridian still works.",
		plural(d.Count, "exercise", "exercises")))
	each := fmt.Sprintf("Each of the %d exercises of the go-practice course", d.Count)
	if d.Count == 1 {
		each = "The exercise of the go-practice course"
	}
	y = checked(c, d, y, each)
	if len(d.Bars) == 0 {
		return drawActivity(c, d, y)
	}
	return drawActivity(c, d, drawSkyline(c, d.Bars, y+9*mm))
}

// drawSkyline draws the course to scale: 1 tower per exercise, as tall as
// its estimate, grouped by tier with the tier's ID under each group, and
// the years apart. Every tower passed, so every tower is ink. It returns
// the baseline of the tier IDs.
func drawSkyline(c *canvas, bars []Bar, y float64) float64 {
	const gapT, gapG, gapY = 1.6, 7.0, 16.0
	year := func(tier int) int { return (tier-1)/5 + 1 }
	tallest := skyMax
	gaps := 0.0
	for i, b := range bars {
		tallest = max(tallest, b.Hours)
		switch {
		case i == 0:
		case year(b.Tier) != year(bars[i-1].Tier):
			gaps += gapY
		case b.Tier != bars[i-1].Tier:
			gaps += gapG
		default:
			gaps += gapT
		}
	}
	tw := (textR - textL - gaps) / float64(len(bars))
	const tall = 18 * mm
	ground := y + 5*mm + tall
	x := textL
	groupX := x
	for i, b := range bars {
		if i == 0 || b.Tier != bars[i-1].Tier {
			if i > 0 {
				c.centre((groupX+x-gapT)/2, ground+11, fmt.Sprintf("T%d", bars[i-1].Tier), fontKind, 8.5, ink2, 0)
				if year(b.Tier) != year(bars[i-1].Tier) {
					// The frame's place for the year line, a little right of
					// the gap's middle, nearer the year it opens.
					lx := x - gapT + (gapG+gapY)/2
					c.line(lx, y-1*mm, lx, ground+4*mm, hair, 0.5, 2, 2)
					x += gapY - gapT
				} else {
					x += gapG - gapT
				}
			}
			if i == 0 || year(b.Tier) != year(bars[i-1].Tier) {
				c.text(x, y+c.capH(fontName, 8.5), fmt.Sprintf("YEAR %d", year(b.Tier)), fontName, 8.5, ink2, 0.85)
			}
			groupX = x
		}
		h := b.Hours / tallest * tall
		c.rect(x, ground-h, tw, h, ink)
		x += tw + gapT
	}
	c.centre((groupX+x-gapT)/2, ground+11, fmt.Sprintf("T%d", bars[len(bars)-1].Tier), fontKind, 8.5, ink2, 0)
	c.line(textL, ground, textR, ground, ink, 1.2)
	return ground + 11
}

// drawActivity is the activity row, only when the owner chose to show
// it: a readout strip, labels over numerals. It returns its bottom.
func drawActivity(c *canvas, d Document, y float64) float64 {
	s := d.Stats
	if s == nil {
		return y
	}
	cells := []struct{ label, value string }{
		{"ACTIVE", duration(s.ActiveSeconds)},
		{"RUNS", fmt.Sprint(s.Runs)},
		{"HINTS", fmt.Sprint(s.Hints)},
		{"EXPLANATIONS", fmt.Sprint(s.Explanations)},
		{"WALKTHROUGHS", fmt.Sprint(s.Walkthroughs)},
		{"SOLUTIONS", fmt.Sprint(s.Solutions)},
	}
	y += 7 * mm
	cw := (textR - textL) / float64(len(cells))
	c.line(textL, y, textR, y, ink, 1.2)
	for i, cell := range cells {
		x := textL + float64(i)*cw
		in := 0.0
		if i > 0 {
			c.line(x, y+3, x, y+11*mm, rule, 0.5)
			in = 2.5 * mm
		}
		c.text(x+in, y+4*mm, cell.label, fontName, 8.5, ink2, 0.85)
		c.text(x+in, y+9.6*mm, cell.value, fontKind, 13, ink, 0)
	}
	y += 11 * mm
	c.line(textL, y, textR, y, rule, 0.5)
	return y
}

// drawFoot draws the issuer line, the QR code with its URL, and the title
// block: the facts of the sheet, like a drawing's, with the certificate ID
// as the drawing number.
func drawFoot(c *canvas, d Document) {
	fy := frameB - footH
	c.line(frameL, fy, frameR, fy, ink, 0.75)
	// The course is the issuer. Vesper Labs is the portal's brand, named
	// only through its address.
	y := fy + 6*mm
	issuer := "Issued by the go-practice course, through Go Practice, the training portal at " + d.Host + "."
	for _, l := range c.wrap(fontText, 9.5, issuer, 62*mm) {
		c.text(frameL+pad, y, l, fontText, 9.5, ink2, 0)
		y += 13
	}

	left := frameR - tbW
	const qrSize = 28 * mm
	qx, qy := left-7*mm-qrSize, fy+4*mm
	drawQR(c, d.VerifyURL, qx, qy, qrSize)
	shown := strings.TrimPrefix(strings.TrimPrefix(d.VerifyURL, "https://"), "http://")
	c.right(qx+qrSize, qy+qrSize+11, shown, fontMono, 8.5, ink, 0)
	c.right(qx+qrSize, qy+qrSize+23, "SCAN TO VERIFY", fontName, 8.5, ink2, 0.85)
	if !c.dry {
		sw := c.measure(fontMono, 8.5, shown)
		c.pdf.AddExternalLink(d.VerifyURL, qx, qy, qrSize, qrSize)
		c.pdf.AddExternalLink(d.VerifyURL, qx+qrSize-sw, qy+qrSize+3, sw, 10)
	}

	for i := 1; i < 4; i++ {
		c.line(left, fy+float64(i)*tbRowH, frameR, fy+float64(i)*tbRowH, ink, 0.75)
	}
	c.line(left, fy, left, frameB, ink, 0.75)
	label := func(x, y float64, s string) { c.text(x+2.8*mm, y, s, fontName, 8.5, ink2, 0.85) }
	cell := func(x, y, w float64, name, value, family string, size float64) {
		label(x, y+3.9*mm, name)
		c.text(x+2.8*mm, y+8.9*mm, value, family, c.fit(family, size, tbMin, value, w-5.6*mm, 0), ink, 0)
	}
	issued := d.IssuedAt.UTC().Format("2 January 2006")
	family := nameFont(c.pdf, d.Name)
	place, lines := titleBlockName(c.measure, family, d.Name, tbW/2-5.6*mm, tbW-5.6*mm)
	switch place {
	case tbHalf:
		mid := left + tbW/2
		c.line(mid, fy, mid, fy+2*tbRowH, ink, 0.75)
		cell(left, fy, tbW/2, "NAME", d.Name, family, 11)
		cell(mid, fy, tbW/2, "ISSUED", issued, fontBold, 11)
		cell(left, fy+tbRowH, tbW/2, "CHECKED BY", "go test -race", fontMono, 10.5)
		cell(mid, fy+tbRowH, tbW/2, "EXERCISES", d.Range, fontBold, 11)
	default:
		// A long name takes the whole first row, and Issued moves into
		// the second, which holds 3 cells.
		if place == tbRow {
			cell(left, fy, tbW, "NAME", d.Name, family, 11)
		} else {
			label(left, fy+3.2*mm, "NAME")
			for i, l := range lines {
				c.text(left+2.8*mm, fy+6.6*mm+float64(i)*9.6, l, family, tbMin, ink, 0)
			}
		}
		third := tbW / 3
		c.line(left+third, fy+tbRowH, left+third, fy+2*tbRowH, ink, 0.75)
		c.line(left+2*third, fy+tbRowH, left+2*third, fy+2*tbRowH, ink, 0.75)
		cell(left, fy+tbRowH, third, "CHECKED BY", "go test -race", fontMono, 10.5)
		cell(left+third, fy+tbRowH, third, "EXERCISES", d.Range, fontBold, 11)
		cell(left+2*third, fy+tbRowH, third, "ISSUED", issued, fontBold, 11)
	}
	cell(left, fy+2*tbRowH, tbW, "CERTIFICATE ID", d.ID, fontMono, 12)
	cell(left, fy+3*tbRowH, tbW-cut, "SIGNING KEY FINGERPRINT", d.Fingerprint, fontMono, 12)
}

// drawQR draws the QR code as filled squares: vector, so it stays sharp
// when printed at any size. Runs of dark modules in a row become 1 rect.
// The paper around the code is its quiet zone.
func drawQR(c *canvas, text string, x, y, size float64) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		c.fail(fmt.Errorf("qr: %w", err))
		return
	}
	mod := size / float64(code.Size)
	for row := range code.Size {
		for col := 0; col < code.Size; {
			if !code.Black(col, row) {
				col++
				continue
			}
			start := col
			for col < code.Size && code.Black(col, row) {
				col++
			}
			// A hair of overlap hides the seams some viewers draw
			// between adjacent rects.
			c.rect(x+float64(start)*mod, y+float64(row)*mod, float64(col-start)*mod+0.05, mod+0.05, ink)
		}
	}
}

// QRMatrix returns the QR code modules for text, row by row; true is dark.
// The PDF draws exactly this matrix.
func QRMatrix(text string) ([][]bool, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	m := make([][]bool, code.Size)
	for y := range m {
		m[y] = make([]bool, code.Size)
		for x := range m[y] {
			m[y][x] = code.Black(x, y)
		}
	}
	return m, nil
}
