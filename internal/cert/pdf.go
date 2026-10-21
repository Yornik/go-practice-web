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
// TrueType outlines and ignores variable-font axes. B612 and Spline Sans
// Mono are the woff2 files of web/static/fonts, decompressed with
// fontTools (".design/FINAL.md", section 13.1).
//
//go:embed fonts
var fontFS embed.FS

// Font families registered in every PDF: the site's 2 families (B612 and
// Spline Sans Mono, converted losslessly from the npm woff2 files), and
// Inter Bold for a name with a letter outside B612's Latin-1 set.
const (
	fontManual     = "manual"      // B612 Regular: text, labels
	fontManualBold = "manual-bold" // B612 Bold: the name, headings, values
	fontMono       = "mono"        // Spline Sans Mono: ID, fingerprint, URL, go test -race
	fontNameWide   = "name-wide"   // Inter Bold: a name B612 cannot set
)

var fontFiles = map[string]string{
	fontManual:     "fonts/b612/B612-Regular.ttf",
	fontManualBold: "fonts/b612/B612-Bold.ttf",
	fontMono:       "fonts/spline-sans-mono/SplineSansMono-Regular.ttf",
	fontNameWide:   "fonts/inter/Inter-Bold.ttf",
}

// Document is what a certificate PDF shows.
type Document struct {
	ID          string
	Kind        Kind
	Name        string
	IssuedAt    time.Time
	Title       string // "Tier 6 · Forensics" or "The complete course"
	Subtitle    string // the tier's theme, or "50 exercises in 10 tiers"
	Tier        int    // the tier a tier certificate covers; 0 for the course or a tier that no longer exists
	Range       string // the exercises it covers, "04 to 09"
	Bars        []Bar  // the course to scale, for the course certificate
	Items       []Item // the tier's exercises, or the course's tiers
	Count       int    // the number of exercises covered
	VerifyURL   string // https://go-practice.dev/verify/GP-...
	Fingerprint string
	Stats       *Stats // nil hides the activity
	Revoked     bool   // stamps the page as revoked
	ReplacedBy  string // the certificate that replaced a revoked one; may be ""
	IssuedAs    string // what a certificate of the first course was issued as (Coverage.IssuedAs); "" otherwise
}

// Bar is 1 exercise of the course strip: as wide as its hours.
type Bar struct {
	Tier  int
	Hours float64
}

// Item is a line of the "what was completed" list.
type Item struct {
	Number string // "04", or "Tier 1"
	Title  string
	Detail string // the activity on it when stats are shown, else ""
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
	d := Document{
		ID:          p.ID,
		Kind:        p.Kind,
		Name:        p.Name,
		IssuedAt:    issued,
		Title:       cv.Title(),
		Range:       cv.Range(),
		VerifyURL:   strings.TrimSuffix(siteURL, "/") + "/verify/" + p.ID,
		Fingerprint: fingerprint,
		Stats:       p.Stats,
		Count:       len(p.Exercises),
		IssuedAs:    cv.IssuedAs(),
	}
	if p.Kind == KindCourse {
		d.Subtitle = cv.Summary()
		covered := map[string]bool{}
		for _, e := range cv.Exercises {
			covered[e.Slug] = true
			d.Bars = append(d.Bars, Bar{Tier: e.Tier, Hours: e.Hours})
		}
		for _, t := range cv.Tiers {
			title := t.Name
			if title == "" {
				title = t.Theme
			}
			it := Item{Number: t.Title(), Title: title}
			if cv.Stats != nil {
				var sum ExerciseStats
				for _, e := range t.Exercises {
					if !covered[e.Slug] {
						continue
					}
					s := cv.Stats[e.Slug]
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
	if cv.Tier != nil {
		d.Tier, d.Subtitle = cv.Tier.N, cv.Tier.Theme
	}
	for _, e := range cv.Exercises {
		it := Item{Number: fmt.Sprintf("%02d", e.Number), Title: e.Title}
		if cv.Stats != nil {
			it.Detail = activityLine(cv.Stats[e.Slug])
		}
		d.Items = append(d.Items, it)
	}
	return d, nil
}

// activityLine is the short stats text under an item.
func activityLine(e ExerciseStats) string {
	return strings.Join([]string{duration(e.ActiveSeconds), plural(e.Runs, "test run", "test runs"), plural(e.Hints, "hint", "hints")}, ", ")
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

// Page geometry: A4 landscape in points. The certificate is a drawing
// sheet: a frame 10 mm in, a drawing area with a 5 mm construction grid,
// and the title block in the bottom right corner.
const (
	pageW = 841.89
	pageH = 595.28
	midX  = pageW / 2
	mm    = 72 / 25.4

	frameIn = 10 * mm
	areaL   = frameIn + 5*mm
	areaR   = pageW - frameIn - 5*mm
	areaT   = 72.0
	areaB   = 372.0
)

type rgb struct{ r, g, b uint8 }

// The colours of ".design/FINAL.md", section 13.1: graphite on white
// paper, so a print does not waste ink; blue pencil only for the brand
// mark and the QR label.
var (
	paper     = rgb{255, 255, 255}
	graphite  = rgb{0x1E, 0x28, 0x21}
	graphite2 = rgb{0x47, 0x5A, 0x4C}
	blue      = rgb{0x23, 0x46, 0xA0}
	gridLine  = rgb{0xDC, 0xE6, 0xD2}
	redline   = rgb{0xB3, 0x25, 0x1A}
	onRed     = rgb{0xF6, 0xF9, 0xF1}
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

func (c *canvas) font(family string, size float64) {
	c.fail(c.pdf.SetFont(family, "", size))
	c.fail(c.pdf.SetCharSpacing(0))
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

// fitFont shrinks the size until s fits in maxW.
func (c *canvas) fitFont(family string, size, minSize float64, s string, maxW float64) {
	for ; size > minSize; size -= 0.5 {
		c.font(family, size)
		if c.width(s) <= maxW {
			return
		}
	}
	c.font(family, minSize)
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

// missing returns the runes of s that a font cannot draw.
func missing(pdf *gopdf.GoPdf, family, s string) []rune {
	if err := pdf.SetFont(family, "", 12); err != nil {
		return []rune(s)
	}
	var out []rune
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if ok, err := pdf.IsCurrFontContainGlyph(r); err != nil || !ok {
			out = append(out, r)
		}
	}
	return out
}

// nameFont is the font of the printed name: B612 Bold when it has every
// letter, otherwise Inter Bold for the whole name, so 1 name never
// mixes 2 fonts. B612 draws "(" and ")" almost like "[" and "]", so a
// name with parentheses is set in Inter too.
func nameFont(pdf *gopdf.GoPdf, name string) string {
	if !strings.ContainsAny(name, "()") && len(missing(pdf, fontManualBold, name)) == 0 {
		return fontManualBold
	}
	return fontNameWide
}

// missingGlyphs returns the runes of s that no name font can draw.
func missingGlyphs(s string) []rune {
	pdf, err := newPDF()
	if err != nil {
		return []rune(s)
	}
	return missing(pdf, nameFont(pdf, s), s)
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
		Subject:      d.Title + " passed by " + d.Name,
		Creator:      "Go Practice",
		Producer:     "go-practice-web",
		CreationDate: d.IssuedAt,
	})
	pdf.AddPage()
	c := &canvas{pdf: pdf}
	c.fillColor(paper)
	pdf.RectFromUpperLeftWithStyle(0, 0, pageW, pageH, "F")
	c.rect(frameIn, frameIn, pageW-2*frameIn, pageH-2*frameIn, 0.75, graphite)
	drawHead(c)
	drawArea(c, d)
	if d.Stats != nil {
		drawActivity(c, *d.Stats)
	}
	drawTitleBlock(c, d)
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

// drawHead puts the brand mark (a balloon with "Go") and the sheet's
// title in the top row of the frame.
func drawHead(c *canvas) {
	const y = 50.0
	c.circle(areaL+9, y-4, 9, 1.2, blue)
	c.font(fontManualBold, 7.5)
	c.textColor(blue)
	c.centered(areaL+9, y-1.4, "Go")
	c.font(fontManualBold, 13)
	c.textColor(graphite)
	c.text(areaL+26, y, "Go Practice")
	c.font(fontManual, 13)
	c.right(areaR, y, "Certificate of completion")
}

// drawArea is the drawing: the construction grid, the name, what was
// passed, and the list of exercises or tiers.
func drawArea(c *canvas, d Document) {
	const sq = 5 * mm
	c.pdf.SetLineWidth(0.25)
	c.strokeColor(gridLine)
	for x := areaL + sq; x < areaR-0.1; x += sq {
		c.pdf.Line(x, areaT, x, areaB)
	}
	for y := areaT + sq; y < areaB-0.1; y += sq {
		c.pdf.Line(areaL, y, areaR, y)
	}
	c.rect(areaL, areaT, areaR-areaL, areaB-areaT, 0.5, graphite)

	// The content block sits in the vertical middle of the area, so a tier
	// with 3 exercises leaves no empty band of grid at the bottom. A tall
	// list may use the room above the name's capitals.
	head := headHeight(d)
	top := areaT + math.Max(-headSlack, ((areaB-areaT)-(head+listHeight(d))-30)/2)

	family := nameFont(c.pdf, d.Name)
	c.fitFont(family, 40, 20, d.Name, areaR-areaL-60)
	c.textColor(graphite)
	c.centered(midX, top+62, d.Name)

	c.font(fontManual, 13)
	c.textColor(graphite)
	what := fmt.Sprintf("passed %s of the go-practice course.", plural(d.Count, "exercise", "exercises"))
	switch {
	case d.Kind == KindCourse:
		what = fmt.Sprintf("passed all %d exercises of the go-practice course.", d.Count)
	case d.Tier > 0:
		what = fmt.Sprintf("passed the %s of tier %d of the go-practice course.", plural(d.Count, "exercise", "exercises"), d.Tier)
	}
	c.centered(midX, top+90, what)
	// "go test -race" is the machine's own words: mono, inside the sentence.
	pre, cmd, post := "Each exercise passed its full test suite with ", "go test -race", "."
	c.font(fontManual, 13)
	wPre, wPost := c.width(pre), c.width(post)
	c.font(fontMono, 12)
	wCmd := c.width(cmd)
	x := midX - (wPre+wCmd+wPost)/2
	c.font(fontManual, 13)
	c.text(x, top+108, pre)
	c.font(fontMono, 12)
	c.text(x+wPre, top+108, cmd)
	c.font(fontManual, 13)
	c.text(x+wPre+wCmd, top+108, post)

	c.fitFont(fontManualBold, 18, 11, d.Title, areaR-areaL-60)
	c.centered(midX, top+144, d.Title)
	if d.Subtitle != "" {
		c.fitFont(fontManual, 11, 8, d.Subtitle, areaR-areaL-60)
		c.textColor(graphite2)
		c.centered(midX, top+161, d.Subtitle)
	}
	if d.IssuedAs != "" {
		// The paper printed when it was issued shows the old numbers; this
		// line says why a new copy differs from it.
		c.fitFont(fontManual, 9, 7, d.IssuedAs, areaR-areaL-60)
		c.textColor(graphite2)
		c.centered(midX, top+headH-2, d.IssuedAs)
	}
	if d.Kind == KindCourse {
		drawTiers(c, d, top+head)
		return
	}
	drawExercises(c, d, top+head)
}

// headHeight is the height of the block above the list, with the line of
// IssuedAs when there is one.
func headHeight(d Document) float64 {
	if d.IssuedAs != "" {
		return headH + issuedH
	}
	return headH
}

// headH is the height of the block above the list: the name, the 2
// sentences, the title and the subtitle. headSlack is how far a tall list
// may move it up, into the empty room above the name's capitals.
const (
	headH     = 186.0
	issuedH   = 20.0
	headSlack = 22.0
)

// listHeight is the height drawTiers or drawExercises takes below its top.
func listHeight(d Document) float64 {
	if d.Kind == KindCourse {
		h := float64(tierRows(d)) * tierRowH
		if len(d.Bars) > 0 {
			h += 2 + 9
		}
		return h
	}
	rowH := 18.0
	if d.Stats != nil {
		rowH = 24
	}
	cols := 2
	if len(d.Items) <= 3 {
		cols = 1
	}
	rows := (len(d.Items) + cols - 1) / cols
	return float64(rows) * rowH
}

// drawExercises lists a tier's exercises: 1 centred column up to 3, else
// 2 columns as wide as the longest title needs.
func drawExercises(c *canvas, d Document, top float64) {
	rowH := 18.0
	if d.Stats != nil {
		rowH = 24
	}
	const numW, gap, maxColW = 24.0, 36.0, 350.0
	c.font(fontManual, 11)
	colW := 0.0
	for _, it := range d.Items {
		colW = math.Max(colW, numW+c.width(it.Title))
	}
	colW = math.Min(colW, maxColW)
	cols := 2
	if len(d.Items) <= 3 {
		cols = 1
	}
	rows := (len(d.Items) + cols - 1) / cols
	left := midX - (float64(cols)*colW+float64(cols-1)*gap)/2
	for i, it := range d.Items {
		col, row := i/rows, i%rows
		x := left + float64(col)*(colW+gap)
		y := top + float64(row)*rowH
		c.font(fontManual, 11)
		c.textColor(graphite2)
		c.text(x, y, it.Number)
		c.fitFont(fontManual, 11, 8, it.Title, colW-numW)
		c.textColor(graphite)
		c.text(x+numW, y, it.Title)
		if it.Detail != "" {
			c.font(fontManual, 7.5)
			c.textColor(graphite2)
			c.text(x+numW, y+10, it.Detail)
		}
	}
}

// tierRowH is the height of 1 row of the course's tier list.
const tierRowH = 17.0

// tierCols is 1 column for up to 5 tiers, else 2.
func tierCols(d Document) int {
	if len(d.Items) <= 5 {
		return 1
	}
	return 2
}

func tierRows(d Document) int {
	cols := tierCols(d)
	return (len(d.Items) + cols - 1) / cols
}

// drawTiers lists the tiers and draws the course to scale: 1 bar per
// exercise, each as wide as its hours, grouped by tier. More than 5 tiers
// go in 2 columns, so 10 tiers take the height of 5.
func drawTiers(c *canvas, d Document, top float64) {
	const numW, gap = 52.0, 40.0
	cols, rows := tierCols(d), tierRows(d)
	colW := 0.0
	c.font(fontManual, 11)
	for _, it := range d.Items {
		colW = math.Max(colW, numW+c.width(it.Title))
	}
	if d.Stats != nil {
		colW = 520
		if cols == 2 {
			colW = (areaR - areaL - 60 - gap) / 2
		}
	}
	colW = math.Min(colW, (areaR-areaL-60-float64(cols-1)*gap)/float64(cols))
	left := midX - (float64(cols)*colW+float64(cols-1)*gap)/2
	for i, it := range d.Items {
		col, row := i/rows, i%rows
		x := left + float64(col)*(colW+gap)
		y := top + float64(row)*tierRowH
		c.font(fontManualBold, 11)
		c.textColor(graphite)
		c.text(x, y, it.Number)
		detailW := 0.0
		if it.Detail != "" {
			c.font(fontManual, 8)
			detailW = c.width(it.Detail) + 10
		}
		c.fitFont(fontManual, 11, 8, it.Title, colW-numW-detailW)
		c.textColor(graphite)
		c.text(x+numW, y, it.Title)
		if it.Detail != "" {
			c.font(fontManual, 8)
			c.textColor(graphite2)
			c.right(x+colW, y, it.Detail)
		}
	}
	if len(d.Bars) == 0 {
		return
	}
	// The strip: a hair between bars of a tier, 8 pt between tiers.
	barGap := 2.0
	if len(d.Bars) > 30 {
		barGap = 1.5
	}
	stripL, stripR := areaL+40, areaR-40
	y := top + float64(rows)*tierRowH + 2
	var total float64
	gaps := 0.0
	for i, b := range d.Bars {
		total += b.Hours
		if i > 0 {
			if b.Tier != d.Bars[i-1].Tier {
				gaps += 8
			} else {
				gaps += barGap
			}
		}
	}
	scale := (stripR - stripL - gaps) / total
	x := stripL
	c.fillColor(graphite)
	for i, b := range d.Bars {
		if i > 0 {
			if b.Tier != d.Bars[i-1].Tier {
				x += 8
			} else {
				x += barGap
			}
		}
		c.pdf.RectFromUpperLeftWithStyle(x, y, b.Hours*scale, 9, "F")
		x += b.Hours * scale
	}
}

// drawActivity is the activity row under the drawing, as a 1-row title
// block: only when the owner chose to show it.
func drawActivity(c *canvas, s Stats) {
	cells := []struct{ label, value string }{
		{"Active", duration(s.ActiveSeconds)},
		{"Runs", fmt.Sprint(s.Runs)},
		{"Hints", fmt.Sprint(s.Hints)},
		{"Explanations", fmt.Sprint(s.Explanations)},
		{"Walkthroughs", fmt.Sprint(s.Walkthroughs)},
		{"Solutions", fmt.Sprint(s.Solutions)},
	}
	const top, h = areaB + 14, 32.0
	x := areaL
	c.font(fontManual, 7)
	widths := make([]float64, len(cells))
	for i, cell := range cells {
		c.font(fontManual, 7)
		lw := c.width(cell.label)
		c.font(fontManualBold, 10)
		widths[i] = math.Max(lw, c.width(cell.value)) + 20
	}
	var total float64
	for _, w := range widths {
		total += w
	}
	c.rect(x, top, total, h, 0.75, graphite)
	for i, cell := range cells {
		if i > 0 {
			c.line(x, top, x, top+h, 0.75, graphite)
		}
		c.font(fontManual, 7)
		c.textColor(graphite2)
		c.text(x+10, top+11, cell.label)
		c.font(fontManualBold, 10)
		c.textColor(graphite)
		c.text(x+10, top+25, cell.value)
		x += widths[i]
	}
}

// drawTitleBlock puts the facts of the certificate in the bottom right
// corner, like the title block of a drawing, with the QR code beside it.
// The certificate ID is the drawing number.
func drawTitleBlock(c *canvas, d Document) {
	const (
		w    = 300.0
		rowH = 30.0
		fpH  = 24.0
	)
	right := areaR
	left := right - w
	bottom := pageH - frameIn - 5*mm
	top := bottom - (3*rowH + fpH)
	c.rect(left, top, w, bottom-top, 0.75, graphite)
	for _, y := range []float64{top + rowH, top + 2*rowH, top + 3*rowH} {
		c.line(left, y, right, y, 0.75, graphite)
	}
	mid := left + w/2
	c.line(mid, top, mid, top+2*rowH, 0.75, graphite)
	cell := func(x, y, maxW float64, label, value, family string, size float64) {
		c.font(fontManual, 7)
		c.textColor(graphite2)
		c.text(x+8, y+10, label)
		c.fitFont(family, size, 6, value, maxW-16)
		c.textColor(graphite)
		c.text(x+8, y+23, value)
	}
	cell(left, top, w/2, "Name", d.Name, nameFont(c.pdf, d.Name), 10)
	cell(mid, top, w/2, "Issued", d.IssuedAt.UTC().Format("2 January 2006"), fontManualBold, 10)
	cell(left, top+rowH, w/2, "Checked by", "go test -race", fontMono, 9.5)
	cell(mid, top+rowH, w/2, "Exercises", d.Range, fontManualBold, 10)
	cell(left, top+2*rowH, w, "Certificate ID", d.ID, fontMono, 11)
	c.font(fontManual, 7)
	c.textColor(graphite2)
	c.text(left+8, top+3*rowH+9, "Key fingerprint")
	c.font(fontMono, 7)
	c.textColor(graphite)
	c.text(left+8, top+3*rowH+19, d.Fingerprint)

	// The QR code left of the title block, 28 mm, with its URL under it.
	const qrSize = 28 * mm
	qrX, qrY := left-14-qrSize, top
	drawQR(c, d.VerifyURL, qrX, qrY, qrSize)
	c.pdf.AddExternalLink(d.VerifyURL, qrX, qrY, qrSize, qrSize)
	c.font(fontManual, 7)
	c.textColor(blue)
	c.right(qrX+qrSize, qrY+qrSize+11, "Scan to verify")
	shown := strings.TrimPrefix(strings.TrimPrefix(d.VerifyURL, "https://"), "http://")
	c.font(fontMono, 8)
	c.textColor(graphite)
	c.right(qrX+qrSize, qrY+qrSize+22, shown)
	c.pdf.AddExternalLink(d.VerifyURL, qrX+qrSize-c.width(shown), qrY+qrSize+14, c.width(shown), 10)
}

// drawRevoked puts a 12 mm band across the top: the certificate is
// revoked, and the band names the one that replaced it.
func drawRevoked(c *canvas, replacedBy string) {
	h := 12 * mm
	c.fillColor(redline)
	c.pdf.RectFromUpperLeftWithStyle(frameIn, frameIn, pageW-2*frameIn, h, "F")
	msg := "Revoked."
	if replacedBy != "" {
		msg = "Revoked. Replaced by " + replacedBy + "."
	}
	c.font(fontManualBold, 13)
	c.textColor(onRed)
	c.centered(midX, frameIn+h/2+4.5, msg)
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
	c.fillColor(graphite)
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
