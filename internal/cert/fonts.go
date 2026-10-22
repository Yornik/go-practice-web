package cert

import (
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/signintech/gopdf"
)

// The fonts are static TrueType instances made by scripts/pdf-fonts.py
// (VESPER.md 13.1; fonts/SOURCES.txt lists the sources and sizes, the
// OFL-*.txt files their licences). gopdf embeds glyf outlines and ignores
// variable axes, reads only a (3,1) format 4 cmap and kerns only from a
// legacy kern table, so the script bakes all 3 into each file. The
// licences travel in the binary with the fonts, as the OFL asks.
//
//go:embed fonts/*.ttf fonts/OFL-*.txt
var fontFS embed.FS

// The font families registered in every PDF.
const (
	fontName  = "name"  // Saira 66 % wide, 760: the name, the tag's stamp, the labels
	fontKind  = "kind"  // Saira 125 % wide, 300, slashed 0: the kind line, numerals, the zones
	fontText  = "text"  // Atkinson Hyperlegible Next 400: sentences
	fontBold  = "bold"  // Atkinson Hyperlegible Next 700: values, "Go Practice"
	fontMono  = "mono"  // Atkinson Hyperlegible Mono 400: the ID, the fingerprint, the URL, go test -race
	fontInter = "inter" // Inter Bold, Latin, Greek and Cyrillic: a name that Saira cannot set
)

var fontFiles = map[string]string{
	fontName:  "fonts/SairaName.ttf",
	fontKind:  "fonts/SairaKind.ttf",
	fontText:  "fonts/AtkinsonNext-Regular.ttf",
	fontBold:  "fonts/AtkinsonNext-Bold.ttf",
	fontMono:  "fonts/AtkinsonMono-Regular.ttf",
	fontInter: "fonts/InterBold-Names.ttf",
}

// fontData is a font file and the 1 metric that gopdf does not expose:
// the cap height as a share of the em, which centres a line of capitals
// in a band and puts the first line of a block on its top edge.
type fontData struct {
	bytes []byte
	cap   float64
}

// loadFonts reads the embedded fonts once; the bytes are shared read only.
var loadFonts = sync.OnceValues(func() (map[string]fontData, error) {
	out := map[string]fontData{}
	for family, path := range fontFiles {
		b, err := fontFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cert: font %s: %w", path, err)
		}
		c, err := capHeight(b)
		if err != nil {
			return nil, fmt.Errorf("cert: font %s: %w", path, err)
		}
		out[family] = fontData{bytes: b, cap: c}
	}
	return out, nil
})

// capHeight reads sCapHeight from the OS/2 table (version 2 and later)
// and divides it by the head table's units per em.
func capHeight(b []byte) (float64, error) {
	table := func(tag string) []byte {
		if len(b) < 12 {
			return nil
		}
		n := int(binary.BigEndian.Uint16(b[4:]))
		for i := range n {
			rec := 12 + 16*i
			if rec+16 > len(b) {
				return nil
			}
			if string(b[rec:rec+4]) != tag {
				continue
			}
			off, size := int(binary.BigEndian.Uint32(b[rec+8:])), int(binary.BigEndian.Uint32(b[rec+12:]))
			if off < 0 || size < 0 || off+size > len(b) {
				return nil
			}
			return b[off : off+size]
		}
		return nil
	}
	head, os2 := table("head"), table("OS/2")
	if len(head) < 20 || len(os2) < 90 || binary.BigEndian.Uint16(os2) < 2 {
		return 0, errors.New("no head table or no OS/2 table of version 2 or later")
	}
	upm := binary.BigEndian.Uint16(head[18:])
	capH := int16(binary.BigEndian.Uint16(os2[88:]))
	if upm == 0 || capH <= 0 {
		return 0, errors.New("no cap height")
	}
	return float64(capH) / float64(upm), nil
}

// newPDF starts an A4 landscape document with every font registered, with
// kerning on. The families go in sorted order, not map order, so the same
// certificate always gives the same bytes.
func newPDF() (*gopdf.GoPdf, map[string]fontData, error) {
	fonts, err := loadFonts()
	if err != nil {
		return nil, nil, err
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: pageH}, Unit: gopdf.UnitPT})
	families := make([]string, 0, len(fonts))
	for family := range fonts {
		families = append(families, family)
	}
	slices.Sort(families)
	for _, family := range families {
		if err := pdf.AddTTFFontDataWithOption(family, fonts[family].bytes, gopdf.TtfOption{UseKerning: true}); err != nil {
			return nil, nil, fmt.Errorf("cert: add font %s: %w", family, err)
		}
	}
	return pdf, fonts, nil
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

// nameFont is the font of the printed name (VESPER.md 13.1.1): Saira Name,
// which stays condensed for long names, when it has every rune; otherwise
// Inter Bold for the whole name, so 1 name never mixes 2 fonts.
func nameFont(pdf *gopdf.GoPdf, name string) string {
	if len(missing(pdf, fontName, name)) == 0 {
		return fontName
	}
	return fontInter
}

// missingGlyphs returns the runes of s that no name font can draw.
func missingGlyphs(s string) []rune {
	pdf, _, err := newPDF()
	if err != nil {
		return []rune(s)
	}
	return missing(pdf, nameFont(pdf, s), s)
}
