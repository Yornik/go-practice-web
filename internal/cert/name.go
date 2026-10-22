package cert

import "sync"

// The name's sizes (VESPER.md 13.1.1): 1 line from nameMax down to
// nameMin; a name that does not fit at nameMin takes 2 lines from nameMin
// down to name2Min.
const (
	nameMax  = 50.0
	nameMin  = 26.0
	name2Min = 18.0
	nameStep = 0.5
	nameLead = 1.04 // the row of a 2-line name, as a share of its size
)

// measurer measures a run of text in a family at a size, with gopdf's own
// arithmetic, so a layout decided here is the layout gopdf draws.
type measurer func(family string, size float64, s string) float64

// nameLayout is how the name sits in a box: its font, its size and its 1
// or 2 lines.
type nameLayout struct {
	family string
	size   float64
	lines  []string
}

// widest is the width of the longest line.
func (l nameLayout) widest(m measurer) float64 {
	w := 0.0
	for _, s := range l.lines {
		w = max(w, m(l.family, l.size, s))
	}
	return w
}

// layoutName fits a name into maxW: on 1 line from nameMax down to
// nameMin, else on 2 lines from nameMin down to name2Min. The 2 lines
// break at a space or after a hyphen; if the longer line still does not
// fit at name2Min, the break moves to the rune that balances the widths.
// So a name of MaxNameRunes never runs out of the frame.
func layoutName(m measurer, family, name string, maxW float64) nameLayout {
	if m(family, nameMin, name) <= maxW {
		return nameLayout{family, fitSize(m, family, nameMax, nameMin, name, maxW), []string{name}}
	}
	var l nameLayout
	for _, anyRune := range []bool{false, true} {
		l = nameLayout{family, nameMin, splitName(m, family, name, nameMin, anyRune)}
		for l.size > name2Min && l.widest(m) > maxW {
			l.size -= nameStep
		}
		if l.widest(m) <= maxW {
			break
		}
	}
	return l
}

// fitSize is the largest size from hi down to lo, in steps of nameStep,
// at which s fits in maxW; lo when none does.
func fitSize(m measurer, family string, hi, lo float64, s string, maxW float64) float64 {
	for size := hi; size > lo; size -= nameStep {
		if m(family, size, s) <= maxW {
			return size
		}
	}
	return lo
}

// splitName breaks a name into 2 lines, the longer as short as it can be,
// measured in the name's own font. The break goes at a space, which is
// dropped, or after a hyphen, which stays on line 1. With anyRune, or
// when the name has neither, any rune boundary counts.
func splitName(m measurer, family, name string, size float64, anyRune bool) []string {
	r := []rune(name)
	var cands [][]string
	if !anyRune {
		for i, c := range r {
			switch {
			case c == ' ':
				cands = append(cands, []string{string(r[:i]), string(r[i+1:])})
			case (c == '-' || c == '‐') && i > 0 && i < len(r)-1:
				cands = append(cands, []string{string(r[:i+1]), string(r[i+1:])})
			}
		}
	}
	if len(cands) == 0 {
		for i := 1; i < len(r); i++ {
			cands = append(cands, []string{string(r[:i]), string(r[i:])})
		}
	}
	if len(cands) == 0 {
		return []string{name}
	}
	widest := func(ls []string) float64 {
		return max(m(family, size, ls[0]), m(family, size, ls[1]))
	}
	// The first of equal candidates wins, so the result does not depend
	// on the order of a sort.
	best := cands[0]
	for _, c := range cands[1:] {
		if widest(c) < widest(best) {
			best = c
		}
	}
	return best
}

// The name in the title block (VESPER.md 13.1.1).
type tbName int

const (
	tbHalf    tbName = iota // in its half cell, beside Issued
	tbRow                   // on 1 line across the whole first row
	tbTwoRows               // on 2 lines across the whole first row
)

// NameTakesTitleRow reports whether the name takes the whole first row of
// the PDF's title block, so that Issued moves into the second row
// (VESPER.md 13.1.1). The verify page repeats the PDF's title block, so it
// asks the PDF's own measure rather than guessing from the length.
func NameTakesTitleRow(name string) (bool, error) {
	f, err := measureName(name)
	return f.row, err
}

// NameInSaira reports whether the PDF sets the name in Saira Name, which
// has every rune of it. Otherwise the PDF sets the whole name in Inter,
// and the verify page sets it in a font that has the runes too, at a size
// that a wider face fits (VESPER.md 13.1.1).
func NameInSaira(name string) (bool, error) {
	f, err := measureName(name)
	return f.saira, err
}

// nameFacts is what the verify page asks the PDF about a name.
type nameFacts struct{ row, saira bool }

// measureName answers both questions with 1 PDF, cached per name.
func measureName(name string) (nameFacts, error) {
	if f, ok := nameCache.Load(name); ok {
		return f.(nameFacts), nil
	}
	// A measure starts a PDF with every font: about 3 ms. The names are
	// those of issued certificates, so the cache stays small.
	pdf, fonts, err := newPDF()
	if err != nil {
		return nameFacts{}, err
	}
	c := &canvas{pdf: pdf, fonts: fonts}
	family := nameFont(pdf, name)
	place, _ := titleBlockName(c.measure, family, name, tbW/2-5.6*mm, tbW-5.6*mm)
	if c.err != nil {
		return nameFacts{}, c.err
	}
	f := nameFacts{row: place != tbHalf, saira: family == fontName}
	nameCache.Store(name, f)
	return f, nil
}

// nameCache caches measureName by name.
var nameCache sync.Map

// titleBlockName places the name in the title block: in its half cell
// when it fits there at tbMin, else across the whole first row (Issued
// moves into the second row), else on 2 lines in that row, broken like
// the name on the sheet.
func titleBlockName(m measurer, family, name string, halfW, rowW float64) (tbName, []string) {
	switch {
	case m(family, tbMin, name) <= halfW:
		return tbHalf, []string{name}
	case m(family, tbMin, name) <= rowW:
		return tbRow, []string{name}
	}
	lines := splitName(m, family, name, tbMin, false)
	if max(m(family, tbMin, lines[0]), m(family, tbMin, lines[len(lines)-1])) > rowW {
		lines = splitName(m, family, name, tbMin, true)
	}
	return tbTwoRows, lines
}
