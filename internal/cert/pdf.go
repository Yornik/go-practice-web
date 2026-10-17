package cert

import (
	"embed"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/signintech/gopdf"
	"rsc.io/qr"

	"github.com/yornik/go-practice-web/internal/course"
)

// The fonts are static OFL instances (see fonts/*/OFL.txt): gopdf embeds
// TrueType outlines and ignores variable-font axes.
//
//go:embed fonts
var fontFS embed.FS

// Font families registered in every PDF.
const (
	fontDisplay       = "display"        // Cormorant Garamond SemiBold: title, date
	fontDisplayItalic = "display-italic" // Cormorant Garamond Medium Italic: the name
	fontSans          = "sans"           // Inter Regular: text
	fontSansBold      = "sans-bold"      // Inter SemiBold: labels
	fontMono          = "mono"           // JetBrains Mono: ID, fingerprint, URL
)

var fontFiles = map[string]string{
	fontDisplay:       "fonts/cormorant-garamond/CormorantGaramond-SemiBold.ttf",
	fontDisplayItalic: "fonts/cormorant-garamond/CormorantGaramond-MediumItalic.ttf",
	fontSans:          "fonts/inter/Inter-Regular.ttf",
	fontSansBold:      "fonts/inter/Inter-SemiBold.ttf",
	fontMono:          "fonts/jetbrains-mono/JetBrainsMono-Regular.ttf",
}

// Document is what a certificate PDF shows.
type Document struct {
	ID          string
	Kind        Kind
	Name        string
	IssuedAt    time.Time
	Heading     string // "Tier 2" or "The complete course"
	Theme       string // the tier's theme, or the course's tagline
	Items       []Item // the tier's exercises, or the course's tiers
	Count       int    // the number of exercises covered
	VerifyURL   string // https://go-practice.dev/verify/GP-...
	Fingerprint string
	Stats       *Stats // nil hides the activity
	Revoked     bool   // stamps the page as revoked
	ReplacedBy  string // the certificate that replaced a revoked one; may be ""
}

// Item is a line of the "what was completed" list.
type Item struct {
	Number string // "04", or "Tier 1"
	Title  string
	Detail string // the activity on it when stats are shown, else ""
}

// NewDocument builds the PDF content from a verified payload, so the PDF
// shows exactly what was signed. siteURL is GPW_PUBLIC_URL.
func NewDocument(c *course.Course, p Payload, siteURL, fingerprint string) (Document, error) {
	issued, err := p.Issued()
	if err != nil {
		return Document{}, err
	}
	d := Document{
		ID:          p.ID,
		Kind:        p.Kind,
		Name:        p.Name,
		IssuedAt:    issued,
		VerifyURL:   strings.TrimSuffix(siteURL, "/") + "/verify/" + p.ID,
		Fingerprint: fingerprint,
		Stats:       p.Stats,
		Count:       len(p.Exercises),
	}
	perExercise := map[string]ExerciseStats{}
	if p.Stats != nil {
		for _, e := range p.Stats.Exercises {
			perExercise[e.Slug] = e
		}
	}
	if p.Kind == KindCourse {
		d.Heading = "The complete course"
		d.Theme = fmt.Sprintf("%d exercises in %d tiers", len(c.Exercises), len(c.Tiers))
		for _, t := range c.Tiers {
			it := Item{Number: fmt.Sprintf("Tier %d", t.N), Title: t.Theme}
			if p.Stats != nil {
				var sum ExerciseStats
				for _, e := range t.Exercises {
					s := perExercise[e.Slug]
					sum.ActiveSeconds += s.ActiveSeconds
					sum.Runs += s.Runs
					sum.Hints += s.Hints
				}
				it.Detail = activityLine(sum)
			}
			d.Items = append(d.Items, it)
		}
		return d, nil
	}
	n, ok := p.Kind.Tier()
	t, ok2 := c.Tier(n)
	if !ok || !ok2 {
		return Document{}, fmt.Errorf("%w: %q", ErrKind, p.Kind)
	}
	d.Heading = fmt.Sprintf("Tier %d", t.N)
	d.Theme = t.Theme
	for _, e := range t.Exercises {
		it := Item{Number: fmt.Sprintf("%02d", e.Number), Title: e.Title}
		if p.Stats != nil {
			it.Detail = activityLine(perExercise[e.Slug])
		}
		d.Items = append(d.Items, it)
	}
	return d, nil
}

// activityLine is the short stats text under an item.
func activityLine(e ExerciseStats) string {
	parts := []string{hours(e.ActiveSeconds), plural(e.Runs, "test run", "test runs"), plural(e.Hints, "hint", "hints")}
	return strings.Join(parts, "  ·  ")
}

func hours(sec int64) string {
	h := float64(sec) / 3600
	if h >= 10 {
		return fmt.Sprintf("%.0f h", math.Round(h))
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", h), ".0") + " h"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Page geometry: A4 landscape in points.
const (
	pageW = 841.89
	pageH = 595.28
	midX  = pageW / 2
)

type rgb struct{ r, g, b uint8 }

// The palette: deep slate ink on warm paper, Go cyan only for the
// ornaments and the seal.
var (
	paper    = rgb{253, 252, 249}
	ink      = rgb{27, 39, 48}
	muted    = rgb{94, 106, 116}
	hairline = rgb{196, 205, 211}
	goCyan   = rgb{0, 173, 216} // #00ADD8
	deepCyan = rgb{0, 125, 156} // #007D9C, for cyan text: enough contrast on paper
	revoked  = rgb{178, 34, 52}
)

// canvas wraps gopdf with a sticky error, so the layout code reads as a
// list of drawing steps.
type canvas struct {
	pdf *gopdf.GoPdf
	err error
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

func (c *canvas) textColor(col rgb)   { c.pdf.SetTextColor(col.r, col.g, col.b) }
func (c *canvas) strokeColor(col rgb) { c.pdf.SetStrokeColor(col.r, col.g, col.b) }
func (c *canvas) fillColor(col rgb)   { c.pdf.SetFillColor(col.r, col.g, col.b) }

func (c *canvas) width(s string) float64 {
	w, err := c.pdf.MeasureTextWidth(s)
	c.fail(err)
	return w
}

// text draws s with its baseline at y.
func (c *canvas) text(x, y float64, s string) {
	c.pdf.SetXY(x, y)
	c.fail(c.pdf.Text(s))
}

func (c *canvas) centered(cx, y float64, s string) { c.text(cx-c.width(s)/2, y, s) }

func (c *canvas) right(x, y float64, s string) { c.text(x-c.width(s), y, s) }

func (c *canvas) line(x1, y1, x2, y2, w float64, col rgb) {
	c.pdf.SetLineWidth(w)
	c.strokeColor(col)
	c.pdf.Line(x1, y1, x2, y2)
}

func (c *canvas) rect(x, y, w, h, lw float64, col rgb) {
	c.pdf.SetLineWidth(lw)
	c.strokeColor(col)
	c.pdf.RectFromUpperLeftWithStyle(x, y, w, h, "D")
}

func (c *canvas) circle(cx, cy, r, lw float64, col rgb) {
	c.pdf.SetLineWidth(lw)
	c.strokeColor(col)
	c.pdf.Oval(cx-r, cy-r, cx+r, cy+r)
}

// diamond draws a filled square on its point, centred at (cx, cy).
func (c *canvas) diamond(cx, cy, r float64, col rgb) {
	c.fillColor(col)
	c.pdf.Polygon([]gopdf.Point{{X: cx, Y: cy - r}, {X: cx + r, Y: cy}, {X: cx, Y: cy + r}, {X: cx - r, Y: cy}}, "F")
}

// fitFont shrinks the size until s fits in maxW.
func (c *canvas) fitFont(family string, size, minSize, spacing float64, s string, maxW float64) {
	for ; size > minSize; size -= 0.5 {
		c.font(family, size, spacing)
		if c.width(s) <= maxW {
			return
		}
	}
	c.font(family, minSize, spacing)
}

// loadFonts reads the embedded fonts once; the bytes are shared read only.
var loadFonts = sync.OnceValues(func() (map[string][]byte, error) {
	data := map[string][]byte{}
	for family, path := range fontFiles {
		b, err := fontFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cert: font %s: %w", path, err)
		}
		data[family] = b
	}
	return data, nil
})

func newPDF() (*gopdf.GoPdf, error) {
	fonts, err := loadFonts()
	if err != nil {
		return nil, err
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: pageH}, Unit: gopdf.UnitPT})
	for family, b := range fonts {
		if err := pdf.AddTTFFontData(family, b); err != nil {
			return nil, fmt.Errorf("cert: add font %s: %w", family, err)
		}
	}
	return pdf, nil
}

// missingGlyphs returns the runes of s that the name font cannot draw.
func missingGlyphs(s string) []rune {
	pdf, err := newPDF()
	if err != nil {
		return []rune(s)
	}
	if err := pdf.SetFont(fontDisplayItalic, "", 12); err != nil {
		return []rune(s)
	}
	var missing []rune
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if ok, err := pdf.IsCurrFontContainGlyph(r); err != nil || !ok {
			missing = append(missing, r)
		}
	}
	return missing
}

// Render writes the certificate as a 1-page PDF.
func Render(w io.Writer, d Document) error {
	pdf, err := newPDF()
	if err != nil {
		return err
	}
	pdf.SetInfo(gopdf.PdfInfo{
		Title:        "Go Practice certificate " + d.ID,
		Author:       "Go Practice",
		Subject:      d.Heading + " completed by " + d.Name,
		Creator:      "go-practice.dev",
		Producer:     "go-practice-web",
		CreationDate: d.IssuedAt,
	})
	pdf.AddPage()
	c := &canvas{pdf: pdf}

	// With stats the page holds 1 more block: the header moves up a little.
	shift := 0.0
	switch {
	case d.Stats != nil && d.Kind == KindCourse:
		shift = -22
	case d.Stats != nil:
		shift = -14
	}
	drawFrame(c)
	drawHeader(c, d, shift)
	bottom := drawItems(c, d, 338+shift)
	if d.Stats != nil {
		drawStats(c, *d.Stats, bottom+14)
	}
	drawFooter(c, d)
	if d.Revoked {
		drawRevoked(c, d.ReplacedBy)
	}
	if c.err != nil {
		return fmt.Errorf("cert: render %s: %w", d.ID, c.err)
	}
	if _, err := pdf.WriteTo(w); err != nil {
		return fmt.Errorf("cert: write %s: %w", d.ID, err)
	}
	return nil
}

// Frame geometry: an outer rule, a guilloche band, an inner rule.
const (
	frameOuter = 20.0
	frameInner = 34.0
)

// drawFrame paints the paper, the double rule with a woven guilloche band
// between the rules, and the corner ornaments.
func drawFrame(c *canvas) {
	c.fillColor(paper)
	c.pdf.RectFromUpperLeftWithStyle(0, 0, pageW, pageH, "F")

	drawBand(c)
	c.rect(frameOuter, frameOuter, pageW-2*frameOuter, pageH-2*frameOuter, 1.2, ink)
	c.rect(frameInner, frameInner, pageW-2*frameInner, pageH-2*frameInner, 0.6, ink)

	// The band's corners hold a square with a diamond, like a stamp.
	const half = (frameInner - frameOuter) / 2
	for _, corner := range [][2]float64{
		{frameOuter + half, frameOuter + half}, {pageW - frameOuter - half, frameOuter + half},
		{frameOuter + half, pageH - frameOuter - half}, {pageW - frameOuter - half, pageH - frameOuter - half},
	} {
		x, y := corner[0], corner[1]
		c.fillColor(paper)
		c.pdf.RectFromUpperLeftWithStyle(x-half, y-half, 2*half, 2*half, "F")
		c.rect(x-half, y-half, 2*half, 2*half, 0.6, ink)
		c.diamond(x, y, 3.6, goCyan)
	}

	// Inner corner brackets.
	const in = 44.0
	for _, corner := range [][2]float64{{in, in}, {pageW - in, in}, {in, pageH - in}, {pageW - in, pageH - in}} {
		x, y := corner[0], corner[1]
		dx, dy := 1.0, 1.0
		if x > midX {
			dx = -1
		}
		if y > pageH/2 {
			dy = -1
		}
		c.line(x, y, x+dx*28, y, 0.45, hairline)
		c.line(x, y, x, y+dy*28, 0.45, hairline)
		c.diamond(x, y, 1.6, hairline)
	}
}

// drawBand fills the space between the rules with phase-shifted sine waves
// that weave into a guilloche, the line work of banknotes and diplomas.
func drawBand(c *canvas) {
	const (
		copies = 3
		period = 13.0 // points per wave along the edge
	)
	mid := (frameOuter + frameInner) / 2
	amp := (frameInner - frameOuter) / 2 * 0.72
	// Slate, not cyan, and translucent: the band frames the page without
	// competing with the text. Cyan stays for the diamonds and the seal.
	c.fail(c.pdf.SetTransparency(gopdf.Transparency{Alpha: 0.55, BlendModeType: gopdf.NormalBlendMode}))
	defer c.pdf.ClearTransparency()
	c.pdf.SetLineWidth(0.22)
	c.strokeColor(muted)
	// Each edge runs between the corner squares.
	edges := []struct{ x0, y0, x1, y1 float64 }{
		{frameInner, mid, pageW - frameInner, mid},
		{frameInner, pageH - mid, pageW - frameInner, pageH - mid},
		{mid, frameInner, mid, pageH - frameInner},
		{pageW - mid, frameInner, pageW - mid, pageH - frameInner},
	}
	for _, e := range edges {
		length := math.Hypot(e.x1-e.x0, e.y1-e.y0)
		ux, uy := (e.x1-e.x0)/length, (e.y1-e.y0)/length
		nx, ny := -uy, ux
		// Whole waves only, so that every edge looks the same at both ends.
		waves := math.Round(length / period)
		steps := int(waves) * 12
		// Each wave runs out along +sin and back along -sin: a curve and
		// its mirror form the chain of lenses that makes the weave, and 1
		// closed path per pair keeps the PDF small. The closing strokes
		// lie on the corner squares' borders, which hide them.
		point := func(t, off float64) gopdf.Point {
			return gopdf.Point{X: e.x0 + ux*length*t + nx*off, Y: e.y0 + uy*length*t + ny*off}
		}
		for k := range copies {
			phase := math.Pi * float64(k) / copies
			pts := make([]gopdf.Point, 2*(steps+1))
			for i := 0; i <= steps; i++ {
				t := float64(i) / float64(steps)
				off := amp * math.Sin(2*math.Pi*waves*t+phase)
				pts[i] = point(t, off)
				pts[len(pts)-1-i] = point(t, -off)
			}
			c.pdf.Polygon(pts, "D")
		}
	}
}

func drawHeader(c *canvas, d Document, shift float64) {
	y := func(v float64) float64 { return v + shift }

	// Brand line, flanked by short rules.
	c.font(fontSansBold, 8.5, 3.4)
	c.textColor(deepCyan)
	brand := "GO PRACTICE"
	bw := c.width(brand) - 3.4 // the last letter's spacing is not visible
	c.text(midX-bw/2, y(84), brand)
	c.line(midX-bw/2-52, y(81), midX-bw/2-14, y(81), 0.5, hairline)
	c.line(midX+bw/2+14, y(81), midX+bw/2+52, y(81), 0.5, hairline)

	c.font(fontDisplay, 44, 0.3)
	c.textColor(ink)
	c.centered(midX, y(142), "Certificate of Completion")

	// Ornament: rule, diamond, rule.
	c.line(midX-74, y(162), midX-10, y(162), 0.5, hairline)
	c.line(midX+10, y(162), midX+74, y(162), 0.5, hairline)
	c.diamond(midX, y(162), 3.6, goCyan)

	c.font(fontDisplayItalic, 17, 0.2)
	c.textColor(muted)
	c.centered(midX, y(198), "This certifies that")

	c.fitFont(fontDisplayItalic, 50, 26, 0.3, d.Name, 620)
	c.textColor(ink)
	c.centered(midX, y(252), d.Name)
	c.line(midX-200, y(267), midX+200, y(267), 0.5, hairline)

	c.font(fontSans, 10.5, 0.1)
	c.textColor(muted)
	what := "has completed every exercise of"
	if d.Kind == KindCourse {
		what = "has completed every exercise of the course"
	}
	c.centered(midX, y(293), what)

	title := d.Heading + "  ·  " + d.Theme
	if d.Kind == KindCourse {
		title = "Go Practice  ·  " + d.Theme
	}
	c.fitFont(fontSansBold, 13.5, 9, 0.2, title, 640)
	c.textColor(ink)
	c.centered(midX, y(314), title)
}

// drawItems lists the exercises (2 columns) or the tiers (a table) from
// top and returns the y below the list.
func drawItems(c *canvas, d Document, top float64) float64 {
	if d.Kind == KindCourse {
		return drawTierTable(c, d, top)
	}
	rowH := 19.0
	if d.Stats != nil {
		rowH = 25
	}
	// The columns are as wide as the longest title needs, so a tier of
	// short titles stays a compact, centred block.
	const numW, gap, maxColW = 21.0, 34.0, 318.0
	c.font(fontSans, 10.5, 0)
	colW := 0.0
	for _, it := range d.Items {
		colW = math.Max(colW, numW+c.width(it.Title))
	}
	colW = math.Min(colW, maxColW)
	// Up to 3 items read best as 1 centred column; more fill 2 columns,
	// the first 1 the longer.
	cols := 2
	if len(d.Items) <= 3 {
		cols = 1
	}
	rows := (len(d.Items) + cols - 1) / cols
	left := midX - (float64(cols)*colW+float64(cols-1)*gap)/2
	y := top + 10
	for i, it := range d.Items {
		col, row := i/rows, i%rows
		x := left + float64(col)*(colW+gap)
		yy := y + float64(row)*rowH
		c.font(fontMono, 8.5, 0)
		c.textColor(deepCyan)
		c.text(x, yy, it.Number)
		c.fitFont(fontSans, 10.5, 8, 0, it.Title, colW-numW)
		c.textColor(ink)
		c.text(x+numW, yy, it.Title)
		if it.Detail != "" {
			c.font(fontSans, 7.5, 0.1)
			c.textColor(muted)
			c.text(x+numW, yy+10.5, it.Detail)
		}
	}
	bottom := y + float64(rows-1)*rowH
	if d.Stats != nil {
		bottom += 10.5
	}
	return bottom
}

// drawTierTable lists the 5 tiers for the course certificate, with the
// activity per tier right-aligned on the same line when stats are shown.
func drawTierTable(c *canvas, d Document, top float64) float64 {
	const numW = 52.0
	rowH := 19.0
	if d.Stats != nil {
		rowH = 17.5
	}
	c.font(fontSans, 10.5, 0)
	titleW := 0.0
	for _, it := range d.Items {
		titleW = math.Max(titleW, c.width(it.Title))
	}
	width := numW + titleW
	if d.Stats != nil {
		width = 560
	}
	left := midX - width/2
	y := top + 10
	for _, it := range d.Items {
		c.font(fontSansBold, 7.5, 1.3)
		c.textColor(deepCyan)
		c.text(left, y, strings.ToUpper(it.Number))
		c.font(fontSans, 10.5, 0)
		c.textColor(ink)
		c.text(left+numW, y, it.Title)
		if it.Detail != "" {
			c.font(fontSans, 8, 0.1)
			c.textColor(muted)
			c.right(left+width, y, it.Detail)
		}
		y += rowH
	}
	return y - rowH
}

// drawStats shows the totals as a row of figures.
func drawStats(c *canvas, s Stats, top float64) {
	figures := []struct{ value, label string }{
		{hours(s.ActiveSeconds), "active time"},
		{fmt.Sprint(s.Runs), "test runs"},
		{fmt.Sprint(s.Hints), "hints"},
		{fmt.Sprint(s.Explanations), "explanations"},
		{fmt.Sprint(s.Walkthroughs), "walkthroughs"},
		{fmt.Sprint(s.Solutions), "solutions"},
	}
	const cellW = 82.0
	left := midX - cellW*float64(len(figures))/2
	for i, f := range figures {
		cx := left + cellW*(float64(i)+0.5)
		// Inter, not Cormorant: Cormorant's old-style 1 reads as an I.
		c.font(fontSans, 14, 0)
		c.textColor(ink)
		c.centered(cx, top+14, f.value)
		c.font(fontSansBold, 5.8, 1.4)
		c.textColor(muted)
		c.centered(cx+0.7, top+25, strings.ToUpper(f.label))
		if i > 0 {
			x := left + cellW*float64(i)
			c.line(x, top+2, x, top+26, 0.4, hairline)
		}
	}
}

func drawFooter(c *canvas, d Document) {
	const base = 478.0
	const leftX = 82.0
	label := func(x, y float64, s string, alignRight bool) {
		c.font(fontSansBold, 6.5, 1.6)
		c.textColor(muted)
		s = strings.ToUpper(s)
		if alignRight {
			c.text(x-c.width(s)+1.6, y, s)
		} else {
			c.text(x, y, s)
		}
	}

	// Left: date and ID.
	label(leftX, base, "Issued", false)
	c.font(fontDisplay, 17, 0.2)
	c.textColor(ink)
	c.text(leftX, base+20, d.IssuedAt.UTC().Format("2 January 2006"))
	label(leftX, base+42, "Certificate ID", false)
	c.font(fontMono, 10.5, 0.4)
	c.textColor(ink)
	c.text(leftX, base+58, d.ID)

	drawSeal(c, d, midX, base+24)

	// Right: QR code and what it proves.
	const qrSize = 72.0
	qrX, qrY := pageW-leftX-qrSize, base-11
	drawQR(c, d.VerifyURL, qrX, qrY, qrSize)
	c.pdf.AddExternalLink(d.VerifyURL, qrX, qrY, qrSize, qrSize)

	tx := qrX - 16
	label(tx, base, "Verify online", true)
	c.font(fontMono, 7.5, 0)
	c.textColor(deepCyan)
	shown := strings.TrimPrefix(strings.TrimPrefix(d.VerifyURL, "https://"), "http://")
	c.right(tx, base+13, shown)
	c.pdf.AddExternalLink(d.VerifyURL, tx-c.width(shown), base+5, c.width(shown), 10)
	label(tx, base+42, "Ed25519 signing key", true)
	c.font(fontMono, 9, 0.3)
	c.textColor(ink)
	c.text(tx-c.width(d.Fingerprint)+0.3, base+57, d.Fingerprint)
}

// drawSeal draws the round seal: a guilloche rosette between 2 rings, and
// the tier number (or the number of exercises) in the middle.
func drawSeal(c *canvas, d Document, cx, cy float64) {
	const rOuter, rInner = 44.0, 27.5
	c.circle(cx, cy, rOuter, 1.4, goCyan)
	c.circle(cx, cy, rOuter-3, 0.35, goCyan)

	// Rosette: circles whose centres run around a ring; their overlaps
	// form the woven band of engraved seals.
	c.pdf.SetLineWidth(0.24)
	c.strokeColor(goCyan)
	const petals = 44
	const ringR, petalR = (rOuter - 3 + rInner) / 2, (rOuter - 3 - rInner) / 2
	for i := range petals {
		a := 2 * math.Pi * float64(i) / petals
		px, py := cx+ringR*math.Cos(a), cy+ringR*math.Sin(a)
		c.pdf.Oval(px-petalR, py-petalR, px+petalR, py+petalR)
	}
	// Paper over the middle, then the inner ring on top.
	c.fillColor(paper)
	c.pdf.Polygon(circlePoints(cx, cy, rInner, 96), "F")
	c.circle(cx, cy, rInner, 0.6, ink)
	c.circle(cx, cy, rInner-2.5, 0.3, hairline)

	label, value := "COURSE", fmt.Sprint(d.Count)
	if n, ok := d.Kind.Tier(); ok {
		label, value = "TIER", fmt.Sprint(n)
	}
	c.font(fontSansBold, 5.5, 1.6)
	c.textColor(deepCyan)
	c.centered(cx+0.8, cy-8, label)
	c.font(fontSansBold, 21, 0)
	c.textColor(ink)
	c.centered(cx, cy+13, value)
}

func circlePoints(cx, cy, r float64, n int) []gopdf.Point {
	pts := make([]gopdf.Point, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = gopdf.Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)}
	}
	return pts
}

// drawQR draws the QR code as filled squares: vector, so it stays sharp
// when printed at any size. Runs of dark modules in a row become 1 rect.
func drawQR(c *canvas, text string, x, y, size float64) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		c.fail(fmt.Errorf("qr: %w", err))
		return
	}
	const quiet = 0 // the paper around the code is the quiet zone
	mod := size / float64(code.Size+2*quiet)
	c.fillColor(ink)
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
			c.pdf.RectFromUpperLeftWithStyle(x+float64(start)*mod, y+float64(row)*mod,
				float64(col-start)*mod+0.05, mod+0.05, "F")
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

// drawRevoked stamps a revoked certificate across the page.
func drawRevoked(c *canvas, replacedBy string) {
	c.fail(c.pdf.SetTransparency(gopdf.Transparency{Alpha: 0.82, BlendModeType: gopdf.NormalBlendMode}))
	c.pdf.Rotate(14, midX, pageH/2)
	c.fillColor(revoked)
	c.pdf.RectFromUpperLeftWithStyle(-80, pageH/2-20, pageW+160, 40, "F")
	c.pdf.SetTextColor(255, 255, 255)
	msg := "REVOKED"
	if replacedBy != "" {
		msg += "  ·  REPLACED BY " + replacedBy
	}
	c.fitFont(fontSansBold, 17, 10, 4, msg, pageW-160)
	c.centered(midX, pageH/2+6, msg)
	c.pdf.RotateReset()
	c.pdf.ClearTransparency()
}
