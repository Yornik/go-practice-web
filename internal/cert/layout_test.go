package cert

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The long names of VESPER.md 13.1.1: 2 of MaxNameRunes runes, and 1 of
// 69 whose only space is near its start, so a break at the space leaves a
// line of 66 runes.
const (
	longLatin    = "Maximiliana Theodora Wilhelmina von Hohenberg-Ostrowska de las Fuentes"
	longCyrillic = "Александра Константиновна Преображенская-Соколовская-Вознесенская Анна"
	longHyphen   = "Ян Константиновна-Преображенская-Соколовская-Вознесенская-Белозерская"
	// No hyphen and 1 space near the start: only a break inside a word fits.
	longNoBreak = "Ян Константиновнапреображенскаясоколовскаявознесенскаябелозерскаяанна"
)

// testCanvas is a canvas on a fresh document, for measuring.
func testCanvas(t *testing.T) *canvas {
	t.Helper()
	pdf, fonts, err := newPDF()
	if err != nil {
		t.Fatal(err)
	}
	pdf.AddPage()
	return &canvas{pdf: pdf, fonts: fonts}
}

func TestLongNameRunes(t *testing.T) {
	for _, n := range []string{longLatin, longCyrillic, longHyphen, longNoBreak} {
		if c := utf8.RuneCountInString(n); c > MaxNameRunes {
			t.Errorf("%q has %d runes, more than MaxNameRunes", n, c)
		}
		if _, err := CleanName(n); err != nil {
			t.Errorf("CleanName(%q): %v", n, err)
		}
	}
	if utf8.RuneCountInString(longLatin) != MaxNameRunes || utf8.RuneCountInString(longCyrillic) != MaxNameRunes || utf8.RuneCountInString(longHyphen) != 69 {
		t.Error("the long names do not have the lengths of VESPER.md 13.1.1")
	}
}

// TestLayoutName checks the size rule of VESPER.md 13.1.1 against the
// numbers the frames measured with gopdf's own arithmetic.
func TestLayoutName(t *testing.T) {
	c := testCanvas(t)
	tests := []struct {
		name   string
		family string
		size   float64
		lines  []string
		widest float64 // 0: not measured by the frames
	}{
		{"Ada Lovelace", fontName, nameMax, []string{"Ada Lovelace"}, 0},
		{"Анна", fontInter, nameMax, []string{"Анна"}, 0},
		{"Łukasz Dvořák", fontName, nameMax, []string{"Łukasz Dvořák"}, 0},
		{longLatin, fontName, 26, []string{"Maximiliana Theodora Wilhelmina von", "Hohenberg-Ostrowska de las Fuentes"}, 325.78},
		{longCyrillic, fontInter, 23.5, nil, 570.42},
		{longHyphen, fontInter, 26, []string{"Ян Константиновна-Преображенская-", "Соколовская-Вознесенская-Белозерская"}, 552.55},
		{longNoBreak, fontInter, 0, nil, 0},
	}
	for _, tt := range tests {
		family := nameFont(c.pdf, tt.name)
		if family != tt.family {
			t.Errorf("%q: font %s, want %s", tt.name, family, tt.family)
		}
		l := layoutName(c.measure, family, tt.name, textR-textL)
		if w := l.widest(c.measure); w > textR-textL {
			t.Errorf("%q: the widest line is %.2f pt, wider than the text column (%.2f pt)", tt.name, w, textR-textL)
		}
		if l.size < name2Min || l.size > nameMax {
			t.Errorf("%q: size %.1f", tt.name, l.size)
		}
		if tt.size != 0 && l.size != tt.size {
			t.Errorf("%q: size %.1f, want %.1f", tt.name, l.size, tt.size)
		}
		if tt.lines != nil && strings.Join(l.lines, "|") != strings.Join(tt.lines, "|") {
			t.Errorf("%q: lines %q, want %q", tt.name, l.lines, tt.lines)
		}
		if tt.widest != 0 && math.Abs(l.widest(c.measure)-tt.widest) > 0.01 {
			t.Errorf("%q: widest %.2f pt, want %.2f", tt.name, l.widest(c.measure), tt.widest)
		}
		// Every rune of the name is on the sheet, in order: a break drops
		// only a space.
		joined := strings.Join(l.lines, "")
		if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(tt.name, " ", "") {
			t.Errorf("%q: lines %q lose runes", tt.name, l.lines)
		}
		if utf8.RuneCountInString(tt.name) > 20 && len(l.lines) != 2 {
			t.Errorf("%q: %d lines, want 2", tt.name, len(l.lines))
		}
	}
}

// TestTitleBlockName checks where the name goes in the title block, and
// that every line stays inside its cell.
func TestTitleBlockName(t *testing.T) {
	c := testCanvas(t)
	half, row := tbW/2-5.6*mm, tbW-5.6*mm
	tests := []struct {
		name  string
		place tbName
	}{
		{"Łukasz Dvořák-Wiśniewska", tbHalf},
		{"Анна", tbHalf},
		{longLatin, tbRow},
		{longCyrillic, tbTwoRows},
		{longHyphen, tbTwoRows},
		{longNoBreak, tbTwoRows},
	}
	for _, tt := range tests {
		family := nameFont(c.pdf, tt.name)
		place, lines := titleBlockName(c.measure, family, tt.name, half, row)
		if place != tt.place {
			t.Errorf("%q: place %d, want %d", tt.name, place, tt.place)
		}
		limit := row
		if place == tbHalf {
			limit = half
		}
		for _, l := range lines {
			if w := c.measure(family, tbMin, l); w > limit {
				t.Errorf("%q: line %q is %.2f pt, wider than its cell (%.2f pt)", tt.name, l, w, limit)
			}
		}
	}
}

// TestSheetFits draws every kind with the longest names, the activity
// row and the first course's line, and checks that the body stays between
// the head and the foot.
func TestSheetFits(t *testing.T) {
	c := loadCourse(t)
	s := testSigner(t)
	for _, k := range Kinds(c) {
		exs, _ := Exercises(c, k)
		var slugs []string
		for _, e := range exs {
			slugs = append(slugs, e.Slug)
		}
		for _, name := range []string{"Ada", longLatin, longCyrillic, longHyphen} {
			for _, revoked := range []bool{false, true} {
				crt, err := s.Issue(Request{Kind: k, Name: name, Exercises: slugs, Stats: BuildStats(slugs, nil), IssuedAt: issuedAt})
				if err != nil {
					t.Fatal(err)
				}
				d, err := NewDocument(c, crt.Payload, "https://vesper-labs.org", s.Fingerprint())
				if err != nil {
					t.Fatal(err)
				}
				d.Revoked, d.ReplacedBy = revoked, "GP-7Q2M-K9XD-3HFA"
				d.IssuedAs = "Issued as Tier 1 of the first course, exercises 01 to 03. The current course numbers them 26 to 28, in Tier 6, Forensics."
				cv := testCanvas(t)
				l, err := drawSheet(cv, d)
				if err != nil {
					t.Fatalf("%s %q: %v", k, name, err)
				}
				if l.contentBottom > l.bodyBottom || l.bodyTop >= l.bodyBottom {
					t.Errorf("%s %q revoked=%v: the body ends at %.1f pt, below the foot at %.1f pt", k, name, revoked, l.contentBottom, l.bodyBottom)
				}
			}
		}
	}
}

// sheetColours renders d without compression and returns the colours the
// page paints with, as gopdf writes them ("1.000 0.667 0.000").
func sheetColours(t *testing.T, d Document) map[string]bool {
	t.Helper()
	pdf, fonts, err := newPDF()
	if err != nil {
		t.Fatal(err)
	}
	pdf.SetNoCompression()
	pdf.AddPage()
	if _, err := drawSheet(&canvas{pdf: pdf, fonts: fonts}, d); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := pdf.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(\d\.\d{3} \d\.\d{3} \d\.\d{3}) (?:rg|RG)\n`).FindAllSubmatch(buf.Bytes(), -1) {
		out[string(m[1])] = true
	}
	return out
}

func pdfColour(c rgb) string {
	f := func(v uint8) string { return strconv.FormatFloat(float64(v)/255, 'f', 3, 64) }
	return f(c.r) + " " + f(c.g) + " " + f(c.b)
}

// TestInks checks that the sheet prints in black and 1 amber, and that a
// revoked sheet carries no amber (VESPER.md 13.1).
func TestInks(t *testing.T) {
	c := loadCourse(t)
	s := testSigner(t)
	inks := map[string]bool{}
	for _, k := range []rgb{paper, ink, ink2, hair, rule, amber} {
		inks[pdfColour(k)] = true
	}
	for _, k := range []Kind{"tier1", KindCourse} {
		exs, _ := Exercises(c, k)
		var slugs []string
		for _, e := range exs {
			slugs = append(slugs, e.Slug)
		}
		crt, err := s.Issue(Request{Kind: k, Name: "Ada Lovelace", Exercises: slugs, IssuedAt: issuedAt})
		if err != nil {
			t.Fatal(err)
		}
		d, err := NewDocument(c, crt.Payload, "https://vesper-labs.org", s.Fingerprint())
		if err != nil {
			t.Fatal(err)
		}
		for _, revoked := range []bool{false, true} {
			d.Revoked = revoked
			used := sheetColours(t, d)
			for col := range used {
				if !inks[col] {
					t.Errorf("%s revoked=%v: the sheet paints with %s, which is not 1 of its inks", k, revoked, col)
				}
			}
			if used[pdfColour(amber)] == revoked {
				t.Errorf("%s revoked=%v: amber on the sheet is %v", k, revoked, used[pdfColour(amber)])
			}
		}
	}
}

// TestSheetText checks the words of the sheet: the kind line, the
// issuer, the line of a tier and of the course, the revoked band.
func TestSheetText(t *testing.T) {
	c := loadCourse(t)
	s := testSigner(t)
	render := func(k Kind, revoked bool) string {
		exs, _ := Exercises(c, k)
		var slugs []string
		for _, e := range exs {
			slugs = append(slugs, e.Slug)
		}
		crt, err := s.Issue(Request{Kind: k, Name: "Ada Lovelace", Exercises: slugs, IssuedAt: issuedAt})
		if err != nil {
			t.Fatal(err)
		}
		d, err := NewDocument(c, crt.Payload, "https://vesper-labs.org/", s.Fingerprint())
		if err != nil {
			t.Fatal(err)
		}
		d.Revoked, d.ReplacedBy = revoked, "GP-7Q2M-K9XD-3HFA"
		cv := testCanvas(t)
		var texts []string
		cv.onText = func(s string) { texts = append(texts, s) }
		if _, err := drawSheet(cv, d); err != nil {
			t.Fatal(err)
		}
		return strings.Join(texts, "\n")
	}
	tier := render("tier1", false)
	for _, want := range []string{
		"TIER CERTIFICATE", "DETAIL A: TAG T1, SCALE 1:1", "T1", "BADGE IN", "01 TO 05",
		"completed Tier 1, Badge In, of the go-practice course: you read the raw lines the dashboards miss.",
		"Each of the 5 exercises passed its full test suite with ", "go test -race",
		"vesper-labs.org/verify/", "SCAN TO VERIFY", "CHECKED BY", "SIGNING KEY FINGERPRINT", "19 October 2026",
	} {
		if !strings.Contains(strings.ReplaceAll(tier, "\n", " "), want) && !strings.Contains(tier, want) {
			t.Errorf("tier sheet: no %q in\n%s", want, tier)
		}
	}
	issuer := "Issued by the go-practice course, through Go Practice, the training portal at vesper-labs.org."
	if !strings.Contains(strings.ReplaceAll(tier, "\n", " "), issuer) {
		t.Errorf("tier sheet: no issuer line %q", issuer)
	}
	if strings.Contains(tier, "REVOKED") || strings.Contains(tier, "·") {
		t.Errorf("tier sheet: a revoked band or a middle dot:\n%s", tier)
	}
	course := render(KindCourse, true)
	for _, want := range []string{
		"COURSE CERTIFICATE", "DETAIL B: THE KEY, SCALE 1:1", "B0", "HALL 3", "YEAR 1", "YEAR 2", "T10",
		"REVOKED", "This certificate was replaced by GP-7Q2M-K9XD-3HFA.",
		"held the Key through 50 exercises and wrote every tool by hand.",
	} {
		if !strings.Contains(course, want) {
			t.Errorf("course sheet: no %q in\n%s", want, course)
		}
	}
	// The sheet names Vesper Labs only through the portal's address: the
	// course is the issuer.
	for _, s := range []string{tier, course} {
		if strings.Contains(s, "Vesper Labs") {
			t.Errorf("the sheet names Vesper Labs:\n%s", s)
		}
	}
}

func TestCapHeight(t *testing.T) {
	_, fonts, err := newPDF()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{fontName: 0.688, fontKind: 0.688, fontText: 0.668, fontBold: 0.668, fontMono: 0.668, fontInter: 1490.0 / 2048}
	for f, w := range want {
		if got := fonts[f].cap; math.Abs(got-w) > 1e-9 {
			t.Errorf("%s: cap height %v, want %v", f, got, w)
		}
	}
	if _, err := capHeight([]byte("not a font")); err == nil {
		t.Error("capHeight read a cap height from junk")
	}
}

// The verify page asks the PDF's measure where the name sits in the
// title block, so the page and the PDF agree (VESPER.md 13.1.1).
func TestNameTakesTitleRow(t *testing.T) {
	for name, want := range map[string]bool{
		"Łukasz Dvořák-Wiśniewska": false,
		"Анна":                     false,
		longLatin:                  true,
		longCyrillic:               true,
	} {
		got, err := NameTakesTitleRow(name)
		if err != nil || got != want {
			t.Errorf("NameTakesTitleRow(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
}

// The verify page sets a name that Saira cannot set in another font, as
// the PDF does (VESPER.md 13.1.1).
func TestNameInSaira(t *testing.T) {
	for name, want := range map[string]bool{
		"Łukasz Dvořák-Wiśniewska": true,
		"Zoë Ngũgĩ":                true,
		"Анна":                     false,
		"Ελένη Παπαδοπούλου":       false,
		longCyrillic:               false,
	} {
		got, err := NameInSaira(name)
		if err != nil || got != want {
			t.Errorf("NameInSaira(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
}

func BenchmarkNameTakesTitleRow(b *testing.B) {
	for b.Loop() {
		if _, err := NameTakesTitleRow(longLatin); err != nil {
			b.Fatal(err)
		}
	}
}
