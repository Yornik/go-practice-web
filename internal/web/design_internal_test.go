package web

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
)

const cssDir = "../../web/static/css"

// cssRule is a style rule: its selector list and the layers it sits in.
type cssRule struct {
	selector string
	layers   []string
}

// cssRules walks a style sheet: comments and strings are skipped, at-rules
// with a block (@layer, @media, @container, @supports) are walked into,
// and @layer names its block's layer. It is enough for the site's own CSS.
func cssRules(t *testing.T, css string) []cssRule {
	t.Helper()
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	var out []cssRule
	var walk func(s string, layers []string)
	walk = func(s string, layers []string) {
		for {
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			open := indexOutsideStrings(s, '{')
			semi := indexOutsideStrings(s, ';')
			if semi >= 0 && (open < 0 || semi < open) {
				s = s[semi+1:] // a statement: @layer a, b; or @import
				continue
			}
			if open < 0 {
				t.Fatalf("css: a rule without a block: %q", s)
			}
			prelude := strings.TrimSpace(s[:open])
			end := matchingBrace(t, s, open)
			body := s[open+1 : end]
			s = s[end+1:]
			switch {
			case strings.HasPrefix(prelude, "@layer"):
				inner := append(append([]string(nil), layers...), strings.TrimSpace(strings.TrimPrefix(prelude, "@layer")))
				walk(body, inner)
			case strings.HasPrefix(prelude, "@media"), strings.HasPrefix(prelude, "@container"), strings.HasPrefix(prelude, "@supports"):
				walk(body, layers)
			case strings.HasPrefix(prelude, "@"):
				// @font-face, @keyframes: no selectors.
			default:
				out = append(out, cssRule{selector: prelude, layers: layers})
			}
		}
	}
	walk(css, nil)
	return out
}

func indexOutsideStrings(s string, c byte) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		switch {
		case quote != 0:
			switch s[i] {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case s[i] == '"' || s[i] == '\'':
			quote = s[i]
		case s[i] == c:
			return i
		}
	}
	return -1
}

func matchingBrace(t *testing.T, s string, open int) int {
	t.Helper()
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		switch {
		case quote != 0:
			switch s[i] {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case s[i] == '"' || s[i] == '\'':
			quote = s[i]
		case s[i] == '{':
			depth++
		case s[i] == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	t.Fatalf("css: unbalanced braces after %q", s[:open])
	return -1
}

var (
	cssAttr  = regexp.MustCompile(`\[[^\]]*\]`)
	cssClass = regexp.MustCompile(`\.(-?[_a-zA-Z][\w-]*)`)
)

// TestStoryLayersUseOurClasses keeps the story components apart from
// everything else (VESPER.md 14.2): every selector in a story sub-layer
// (components.story, pages.story, ide.story) names at least 1 class, and
// every class it names starts with gpw-. So a story rule cannot reach a
// re-skinned component, a bare element or Monaco's DOM.
func TestStoryLayersUseOurClasses(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(cssDir, "*.css"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no CSS files in %s: %v", cssDir, err)
	}
	story := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range cssRules(t, string(b)) {
			inStory := false
			for _, l := range r.layers {
				if strings.HasSuffix(l, ".story") || l == "story" {
					inStory = true
				}
			}
			if !inStory {
				continue
			}
			for _, sel := range strings.Split(r.selector, ",") {
				story++
				classes := cssClass.FindAllStringSubmatch(cssAttr.ReplaceAllString(sel, ""), -1)
				if len(classes) == 0 {
					t.Errorf("%s: story selector %q names no class", filepath.Base(f), strings.TrimSpace(sel))
				}
				for _, c := range classes {
					if !strings.HasPrefix(c[1], "gpw-") {
						t.Errorf("%s: story selector %q uses the class %q", filepath.Base(f), strings.TrimSpace(sel), c[1])
					}
				}
			}
		}
	}
	if story < 50 {
		t.Errorf("found %d story selectors; the parser lost the components.story layer of site.css", story)
	}
}

// TestCSSRulesParser keeps the walker of TestStoryLayersUseOurClasses
// honest about nesting, strings and statements.
func TestCSSRulesParser(t *testing.T) {
	css := `@layer tokens, base; /* a { } */
@layer components.story { .gpw-a, .gpw-b::after { content: "}"; } @media (max-width: 1px) { .gpw-c .x { color: red; } } }
.top { background: url("data:image/svg+xml,%3Csvg%3E{%3C/svg%3E"); }`
	got := cssRules(t, css)
	want := []cssRule{
		{".gpw-a, .gpw-b::after", []string{"components.story"}},
		{".gpw-c .x", []string{"components.story"}},
		{".top", nil},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("cssRules = %v, want %v", got, want)
	}
}

// TestSkylineScale checks the fixed scale of site.css that the skyline's
// classes name: every tower height, every plate offset and every group
// width that skyline can write.
func TestSkylineScale(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(cssDir, "site.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for v := skyMinTenths; v <= skyFullTenths; v++ {
		pct := fmt.Sprintf("%.3f%%", float64(v)/skyFullTenths*100)
		for _, want := range []string{
			fmt.Sprintf(".gpw-h%d { height: %s; }", v, pct),
			fmt.Sprintf(".gpw-g-top%d > .gpw-sky-plate { bottom: calc(%s + 16px); }", v, pct),
		} {
			if !strings.Contains(css, want) {
				t.Errorf("site.css lacks %q", want)
			}
		}
	}
	for n := 1; n <= skyMaxGroup; n++ {
		if want := fmt.Sprintf(".gpw-g%d { flex: %d 1 0; }", n, n); !strings.Contains(css, want) {
			t.Errorf("site.css lacks %q", want)
		}
	}
	// The scale must not take the names of the headlines and the grid.
	for _, name := range []string{".gpw-h1 { height", ".gpw-h2 { height", ".gpw-g12 { flex"} {
		if strings.Contains(css, name) {
			t.Errorf("site.css has %q", name)
		}
	}
}

// testCourse is a small course with 2 years of tiers.
func testCourse() *course.Course {
	c := &course.Course{}
	n := 0
	for tier := 1; tier <= 7; tier++ {
		tr := &course.Tier{N: tier, Name: fmt.Sprintf("Tier name %d", tier)}
		for i := 0; i < 2+tier%3; i++ {
			n++
			e := &course.Exercise{Number: n, Slug: fmt.Sprintf("%02d-ex", n), Tier: tier, Hours: float64(1+i) * 0.5 * float64(tier%4+1)}
			tr.Exercises = append(tr.Exercises, e)
			c.Exercises = append(c.Exercises, e)
		}
		c.Tiers = append(c.Tiers, tr)
	}
	return c
}

func TestSkyline(t *testing.T) {
	c := testCourse()
	states := map[string]string{"01-ex": "passed", "02-ex": "passed", "03-ex": "passed", "04-ex": "here", "05-ex": "started"}
	v, err := skylineFunc(c, states, "dash", "Your skyline")
	if err != nil {
		t.Fatal(err)
	}
	if v.Class != "gpw-sky-dash" || v.Label != "Your skyline" || v.Mini {
		t.Errorf("view = %+v", v)
	}
	if len(v.Years) != 2 || len(v.Years[0].Tiers) != 5 || len(v.Years[1].Tiers) != 2 {
		t.Fatalf("years = %+v", v.Years)
	}
	t1, t2 := v.Years[0].Tiers[0], v.Years[0].Tiers[1]
	if t1.Plate != "on" || t2.Plate != "cur" || v.Years[0].Tiers[2].Plate != "" {
		t.Errorf("plate states = %q %q %q", t1.Plate, t2.Plate, v.Years[0].Tiers[2].Plate)
	}
	towers := 0
	for _, y := range v.Years {
		for _, tr := range y.Tiers {
			roofs := 0
			for _, tw := range tr.Towers {
				towers++
				if tw.Roof {
					roofs++
					if tw.H != tr.Top {
						t.Errorf("tier %d: the roof is on a tower of %d, the tallest is %d", tr.N, tw.H, tr.Top)
					}
				}
				if tw.H < skyMinTenths || tw.H > skyFullTenths {
					t.Errorf("tower %d: %d tenths is off the scale", tw.Number, tw.H)
				}
			}
			if roofs != 1 || tr.Size() != len(tr.Towers) {
				t.Errorf("tier %d: %d roofs, size %d for %d towers", tr.N, roofs, tr.Size(), len(tr.Towers))
			}
		}
	}
	if towers != len(c.Exercises) {
		t.Errorf("%d towers for %d exercises", towers, len(c.Exercises))
	}
	// Tier 1 holds 01 to 03, tier 2 starts with 04.
	if got := v.Years[0].Tiers[1].Towers[0].State; got != "cur" {
		t.Errorf("04 is %q", got)
	}
	if got := v.Years[0].Tiers[1].Towers[1].State; got != "" {
		t.Errorf("a started exercise is %q, want an outline", got)
	}
	if _, err := skylineFunc(c, nil, "poster", ""); err == nil {
		t.Error("an unknown kind is not an error")
	}
	for h, want := range map[float64]int{0.25: skyMinTenths, 1: 10, 1.5: 15, 2.5: 25, 6: 60, 8: 60} {
		if got := skyTenths(h); got != want {
			t.Errorf("skyTenths(%v) = %d, want %d", h, got, want)
		}
	}
}

// TestSkylineCourse keeps every estimate of the real course on the scale,
// so no tower is cut to fit.
func TestSkylineCourse(t *testing.T) {
	c, err := course.Load(cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course")))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Exercises {
		if tenths := e.Hours * 10; tenths < skyMinTenths || tenths > skyFullTenths || tenths != float64(int(tenths)) {
			t.Errorf("%s: %v h is not a tenth of an hour from 0.5 to 6 h", e.Slug, e.Hours)
		}
	}
	for _, tr := range c.Tiers {
		if len(tr.Exercises) > skyMaxGroup {
			t.Errorf("tier %d has %d exercises; the scale has groups up to %d", tr.N, len(tr.Exercises), skyMaxGroup)
		}
	}
}

func TestZero(t *testing.T) {
	z := `<span class="gpw-z">0</span>`
	for in, want := range map[any]string{
		"06/50":       `<span class="gpw-zs">` + z + `6/5` + z + `</span>`,
		"T10":         `<span class="gpw-zs">T1` + z + `</span>`,
		"40\u00a0000": `<span class="gpw-zs">4` + z + "\u00a0" + z + z + z + `</span>`,
		"2026":        `<span class="gpw-zs">2` + z + `26</span>`,
		"T1 <b>":      `<span class="gpw-zs">T1 &lt;b&gt;</span>`,
		"Oslo":        `<span class="gpw-zs">Oslo</span>`,
		10:            `<span class="gpw-zs">1` + z + `</span>`,
	} {
		if got := string(zero(in)); got != want {
			t.Errorf("zero(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDict(t *testing.T) {
	m, err := dict("N", 2, "Name", "The Key")
	if err != nil || m["N"] != 2 || m["Name"] != "The Key" {
		t.Errorf("dict = %v, %v", m, err)
	}
	if _, err := dict("N"); err == nil {
		t.Error("an odd count is not an error")
	}
	if _, err := dict(1, 2); err == nil {
		t.Error("a key that is not a string is not an error")
	}
}

func TestInitialsAndBadge(t *testing.T) {
	for name, want := range map[string]string{
		"Ada Lovelace": "AL", "Ada Byron Lovelace": "AB", "Łukasz Dvořák-Wiśniewska": "ŁD", "Cher": "C", "  ": "?",
	} {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
	t1 := &tierTag{Tier: &course.Tier{N: 1, Name: "Badge In"}}
	for _, tc := range []struct {
		b                staffBadge
		label, menu, tag string
	}{
		{newBadge("Ada Lovelace", t1, false), "Ada Lovelace (AL), highest tag T1, Badge In. Account menu.", "Highest tag T1, Badge In", "T1"},
		{newBadge("Ada Lovelace", nil, false), "Ada Lovelace (AL), day 1, no tags yet. Account menu.", "Day 1, no tags yet", "Day 1"},
		// The Key outranks every tag; it never reads "Day 1".
		{newBadge("Анна", nil, true), "Анна (А), holds the Key, the course certificate. Account menu.", "Holds the Key, the course certificate", "Key"},
		{newBadge("Ada Lovelace", t1, true), "Ada Lovelace (AL), holds the Key, the course certificate. Account menu.", "Holds the Key, the course certificate", "Key"},
		// Without the tags the badge shows no tag rather than a wrong one.
		{unknownBadge("Ada Lovelace"), "Ada Lovelace (AL). Account menu.", "", ""},
	} {
		if tc.b.Label != tc.label || tc.b.Menu != tc.menu || tc.b.TagText() != tc.tag {
			t.Errorf("badge = %q %q %q, want %q %q %q", tc.b.Label, tc.b.Menu, tc.b.TagText(), tc.label, tc.menu, tc.tag)
		}
	}
}

// TestStoryPartials renders the shared story partials: no style
// attribute (the CSP), gpw- classes only, and the 0 of every Saira
// numeral slashed.
func TestStoryPartials(t *testing.T) {
	r, err := newRenderer(os.DirFS("../../web/templates"), false, func(p string) string { return "/static/" + p })
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := r.template("landing")
	if err != nil {
		t.Fatal(err)
	}
	sky, err := skylineFunc(testCourse(), map[string]string{"01-ex": "passed", "02-ex": "here"}, "landing", "The course")
	if err != nil {
		t.Fatal(err)
	}
	m := func(pairs ...any) map[string]any {
		d, err := dict(pairs...)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cases := map[string]struct {
		name string
		data any
		want []string
	}{
		"tag":            {"tag", m("N", 10, "Name", "The Key Turns", "State", "on"), []string{`class="gpw-tag gpw-tag-on"`, `T1<span class="gpw-z">0</span> · THE KEY TURNS`}},
		"telem landing":  {"telem-landing", nil, []string{`aria-label="Where you are"`, `<b><span class="gpw-zs">floor 61</span></b>`, `<i>HUSH</i>, for <span class="gpw-zs">4<span class="gpw-z">0</span>`}},
		"telem app":      {"telem-app", readout{Ledger: "Ada Lovelace, junior SRE", Month: "October", Tier: 2, KeyOnRing: true}, []string{"Ada Lovelace, junior SRE</span>", "October</span>, <b>", "<b>On call</b>, with Wren"}},
		"telem day 1":    {"telem-app", readout{Ledger: "Ada, junior SRE", Month: "September", Tier: 1}, []string{"The Key is on its hook"}},
		"telem no month": {"telem-app", readout{Ledger: "Ada, SRE", Tier: 11}, []string{"<dd><b><span class=\"gpw-zs\">tier 11</span></b></dd>"}},
		"calm lobby":     {"calm", m("Kind", "lobby", "Level", 3), []string{`class="gpw-calm gpw-calm-lobby gpw-story"`, `<h3 id="calm-h"`, "99.2", "All 40 000 customers are calm. Please rest."}},
		"calm strip":     {"calm", m("Kind", "strip", "Level", 2, "After50", false), []string{`<h2 id="calm-h"`, "It said 99.2 on your first day too.", `99.<span class="gpw-z">0</span>`, `3<span class="gpw-z">0</span> nights ago`}},
		"calm after 50":  {"calm", m("Kind", "strip", "Level", 2, "After50", true), []string{">71<", "Lower, and true. Every pager in Meridian still works."}},
		"key":            {"key", "gpw-keybig", []string{`<svg class="icon gpw-keybig" aria-hidden="true"><use href="#i-key"/></svg>`}},
		"skyline":        {"skyline", sky, []string{`class="gpw-sky gpw-sky-refl" role="img" aria-label="The course"`, "gpw-tw-on", "gpw-tw-cur", "gpw-sky-plate-cur", "gpw-tw-roof", "Year 2"}},
	}
	for label, tc := range cases {
		var b bytes.Buffer
		if err := tmpl.ExecuteTemplate(&b, tc.name, tc.data); err != nil {
			t.Errorf("%s: %v", label, err)
			continue
		}
		out := b.String()
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s lacks %q in:\n%s", label, w, out)
			}
		}
		if strings.Contains(out, "style=") {
			t.Errorf("%s has a style attribute", label)
		}
		for _, cls := range regexp.MustCompile(`class="([^"]*)"`).FindAllStringSubmatch(out, -1) {
			for _, c := range strings.Fields(cls[1]) {
				if !strings.HasPrefix(c, "gpw-") && c != "icon" {
					t.Errorf("%s uses the class %q", label, c)
				}
			}
		}
	}
	// The skyline's plates sit inside a picture: no link, no button.
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, "skyline", sky); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "<a ") || strings.Contains(b.String(), "<button") {
		t.Error("the skyline holds a control")
	}
}
