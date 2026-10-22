package web

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

// The page data of the learner's pages that only their templates read:
// the dashboard, the certificates, the verify page, the account and admin.

// MarkLines is the slab's mark label on 2 lines, broken after its comma:
// "Breaker 0," over "Hall 3" (VESPER.md 6.23). It is nil without a label.
func (v slab) MarkLines() []string {
	if v.MarkLabel == "" {
		return nil
	}
	lines := strings.SplitAfterN(v.MarkLabel, ", ", 2)
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return lines
}

// CertClass is the tier's row class on the certificates page: held,
// ready to claim, in progress or not started (VESPER.md 8.7).
func (t *tierTag) CertClass() string {
	switch t.PlateWord() {
	case "Held":
		return "gpw-ct-held"
	case "Ready to claim":
		return "gpw-ct-claim"
	case "In progress":
		return "gpw-ct-progress"
	}
	return "gpw-ct-blank"
}

// Span is the tier's exercises for a sentence: "40 to 44".
func (t *tierTag) Span() string { return fmt.Sprintf("%02d to %02d", t.First, t.Last) }

// PlateSVG is the tier's tag as the certificates page hangs it from the
// tier rule: 112 by 156 units at 1:1, so its stamp stays 12 px. Solid
// amber means stamped and nothing else: a tier ready to claim is a plain
// plate in a 2 px amber keyline, a tier in progress a dashed amber
// outline, the rest grey (VESPER.md 6.18). Ported from plate_svg in
// .design/vesper-v2/tools/build_pages.py.
func (t *tierTag) PlateSVG() template.HTML {
	var body, ink, hole, rule string
	switch t.CertClass() {
	case "gpw-ct-held":
		body, ink, hole, rule = "gpw-sv-plate", " gpw-sv-t-plate", "gpw-sv-punch gpw-sv-punch-ink", "gpw-sv-plate-rule"
	case "gpw-ct-claim":
		body, ink, hole, rule = "gpw-sv-claim", " gpw-sv-t-ink", "gpw-sv-punch gpw-sv-punch-am", "gpw-sv-h2"
	case "gpw-ct-progress":
		body, ink, hole, rule = "gpw-sv-am gpw-sv-dash", " gpw-sv-t-am", "gpw-sv-punch gpw-sv-punch-am", "gpw-sv-h"
	default:
		body, ink, hole, rule = "gpw-sv-h2", "", "gpw-sv-punch", "gpw-sv-h"
	}
	var b strings.Builder
	b.WriteString(`<svg class="gpw-ct-plate" viewBox="0 0 112 156" aria-hidden="true"><path class="gpw-sv-h2" d="M56 0V19"/>`)
	// Open corner brackets at 3 corners; the 4th is the chamfer.
	b.WriteString(`<path class="gpw-sv-h2" d="M12 9H3V18"/><path class="gpw-sv-h2" d="M100 9H109V18"/><path class="gpw-sv-h2" d="M12 151H3V142"/>`)
	fmt.Fprintf(&b, `<path class="%s" d="M8 14H104V132L90 146H8Z"/><circle class="%s" cx="56" cy="25" r="5"/>`, body, hole)
	fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-tn%s" x="56" y="80" text-anchor="middle">%s</text>`, ink, zeroSV(t.Tag()))
	if t.Name != "" {
		fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-ts%s" x="56" y="101" text-anchor="middle">%s</text>`, ink, zeroSV(strings.ToUpper(t.Name)))
	}
	fmt.Fprintf(&b, `<path class="%s" d="M22 111H90"/>`, rule)
	fmt.Fprintf(&b, `<text class="gpw-sv-t gpw-sv-ts%s" x="56" y="129" text-anchor="middle">%s</text></svg>`, ink, zeroSV(strings.ToUpper(t.Span())))
	// Every value is a number, a fixed string or escaped by zeroSV.
	return template.HTML(b.String()) //nolint:gosec // see above
}

// KeyArt is the Key on the certificates page's rail: hung from the rail
// on a small ring with its B0 tag, 200 by 236 units. Dashed while it is
// not held, a plain plate in an amber keyline when it is ready to claim,
// solid amber when it is held (VESPER.md 6.28).
func (s *story) KeyArt() template.HTML {
	class := "gpw-ct-keyg"
	switch {
	case s.Key.Held != nil:
		class = "gpw-ct-keyg gpw-ct-keyg-held"
	case s.Key.Ready:
		class = "gpw-ct-keyg gpw-ct-keyg-ready"
	}
	svg := `<svg class="gpw-ct-keysvg" viewBox="0 0 200 236" aria-hidden="true"><path class="gpw-sv-h2" d="M100 0V19"/>` +
		`<circle class="gpw-ct-ring" cx="100" cy="30" r="11"/><path class="gpw-sv-cord" d="M88 42L60 66"/>` +
		`<g transform="translate(60 66) rotate(-6)"><path class="gpw-sv-b0" d="M-36 -6H6V16L0 22H-36Z"/><circle class="gpw-sv-b0h" cx="0" cy="0" r="2.4"/>` +
		`<text class="gpw-sv-t-b0" x="-16" y="13" text-anchor="middle">B<tspan class="gpw-sv-z">0</tspan></text></g>` +
		`<g transform="translate(100 41) scale(2.6) translate(0 5)"><path class="` + class + `" fill-rule="evenodd" d="` + keyPath + `"/></g></svg>`
	return template.HTML(svg) //nolint:gosec // fixed strings only
}

// KeySummary is the claim dialog's line for the Key: "The Key covers
// exercises 01 to 50. All 50 pass."
func (s *story) KeySummary() string {
	return fmt.Sprintf("The Key covers exercises 01 to %02d. All %d pass.", s.Total, s.Total)
}

// Art is the paper's drawing (VESPER.md 8.9, 13.1): a tier certificate's
// tag on its ring with centre lines, or the Key on its ring with its B0
// tag, 164 by 238 units. A revoked sheet carries no amber: the plate turns
// to paper. Ported from paper_art in .design/vesper-v2/tools/build_pages.py.
func (v verifyView) Art() template.HTML {
	plate := "gpw-pp-plate"
	if v.Status == "revoked" {
		plate += " gpw-pp-plate-void"
	}
	var b strings.Builder
	b.WriteString(`<svg class="gpw-pp-art" viewBox="0 0 164 238" aria-hidden="true"><circle class="gpw-pp-ring" cx="80" cy="24" r="18"/>`)
	if v.IsCourse {
		// The Key hangs from the ring by its bow; B0 hangs from the bow on its cord.
		b.WriteString(`<path class="gpw-pp-ln" d="M17 54H4V67"/><path class="gpw-pp-ln" d="M143 54H156V67"/><path class="gpw-pp-ln" d="M17 230H4V217"/><path class="gpw-pp-ln" d="M143 230H156V217"/>`)
		b.WriteString(`<path class="gpw-pp-ghost" d="M147 58H160V71"/><path class="gpw-pp-ghost" d="M21 234H8V221"/><path class="gpw-pp-ghost" d="M147 234H160V221"/>`)
		b.WriteString(`<path class="gpw-pp-ln" d="M62 72L36 98"/>`)
		// The tag's lines stay at 11 px or more: the art is drawn 1:1.
		b.WriteString(`<g transform="translate(36 98) rotate(-6)"><path class="gpw-pp-b0" d="M-34 -6H8V28L2 34H-34Z"/><circle class="gpw-pp-hole" cx="2" cy="0" r="2.4"/>` +
			`<text class="gpw-pp-ts" x="-14" y="12" text-anchor="middle">B<tspan class="gpw-sv-z">0</tspan></text><text class="gpw-pp-ts gpw-pp-ts-s" x="-13" y="27" text-anchor="middle">HALL 3</text></g>`)
		fmt.Fprintf(&b, `<g transform="translate(80 58) scale(2.3)"><path class="%s" fill-rule="evenodd" d="%s"/></g>`, plate, keyPath)
		b.WriteString(`<path class="gpw-pp-ctr" d="M80 30V214"/></svg>`)
		return template.HTML(b.String()) //nolint:gosec // fixed strings only
	}
	b.WriteString(`<path class="gpw-pp-ln" d="M80 42V62"/>`)
	b.WriteString(`<path class="gpw-pp-ln" d="M17 54H4V67"/><path class="gpw-pp-ln" d="M143 54H156V67"/><path class="gpw-pp-ln" d="M17 230H4V217"/><path class="gpw-pp-ln" d="M143 230H156V217"/>`)
	b.WriteString(`<path class="gpw-pp-ghost" d="M147 58H160V71"/><path class="gpw-pp-ghost" d="M21 234H8V221"/><path class="gpw-pp-ghost" d="M147 234H160V221"/>`)
	fmt.Fprintf(&b, `<path class="%s" d="M12 62H148V200L126 222H12Z"/>`, plate)
	b.WriteString(`<path class="gpw-pp-rim" d="M17 67H143V198L124 217H17Z"/><path class="gpw-pp-ctr" d="M80 56V104M58 82H102"/><circle class="gpw-pp-hole" cx="80" cy="82" r="6.5"/>`)
	fmt.Fprintf(&b, `<text class="gpw-pp-tn" x="80" y="152" text-anchor="middle">%s</text>`, zeroSV(fmt.Sprintf("T%d", v.TierN)))
	if v.Tier != nil && v.Tier.Name != "" {
		fmt.Fprintf(&b, `<text class="gpw-pp-ts" x="80" y="176" text-anchor="middle">%s</text>`, zeroSV(strings.ToUpper(v.Tier.Name)))
	}
	fmt.Fprintf(&b, `<path class="gpw-pp-ln" d="M40 187H120"/><text class="gpw-pp-ts" x="80" y="207" text-anchor="middle">%s</text></svg>`, zeroSV(strings.ToUpper(v.Range)))
	// Every value is a number, a fixed string or escaped by zeroSV.
	return template.HTML(b.String()) //nolint:gosec // see above
}

// What is the paper's line after the name, as the PDF sets it: "completed
// Tier 1, Badge In, of the go-practice course: you read the raw lines the
// dashboards miss." (internal/cert drawTierText, drawCourseText).
func (v verifyView) What() string {
	if v.IsCourse {
		return fmt.Sprintf("held the Key through %d %s and wrote every tool by hand. Every pager in Meridian still works.",
			len(v.Exercises), pluralWord(len(v.Exercises), "exercise", "exercises"))
	}
	what := fmt.Sprintf("Tier %d", v.TierN)
	line := ""
	if v.Tier != nil {
		if v.Tier.Name != "" {
			what += ", " + v.Tier.Name + ","
		}
		line = v.Tier.Line()
	}
	if line == "" {
		return "completed " + what + " of the go-practice course."
	}
	return "completed " + what + " of the go-practice course: " + line + "."
}

// Each starts the paper's check line: "Each of the 5 exercises".
func (v verifyView) Each() string {
	n := len(v.Exercises)
	switch {
	case v.IsCourse && n == 1:
		return "The exercise of the go-practice course"
	case v.IsCourse:
		return fmt.Sprintf("Each of the %d exercises of the go-practice course", n)
	case n == 1:
		return "The exercise"
	}
	return fmt.Sprintf("Each of the %d exercises", n)
}

// AlsoRevealed names the help revealed on an exercise beyond the hints:
// "explanation, walkthrough", or "nothing".
func (v verifyView) AlsoRevealed(slug string) string {
	e := v.StatsBySlug[slug]
	var out []string
	if e.Explanation {
		out = append(out, "explanation")
	}
	if e.Walkthrough {
		out = append(out, "walkthrough")
	}
	if e.Solution {
		out = append(out, "solution")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, ", ")
}

// host is the site's address without its scheme: "vesper-labs.org".
func (s *Server) host() string {
	if u, err := url.Parse(s.origin); err == nil && u.Host != "" {
		return u.Host
	}
	return s.origin
}
