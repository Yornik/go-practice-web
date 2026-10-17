package web

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// strip is the course to scale: 25 bars as wide as their hours, grouped
// by tier (".design/FINAL.md", section 6.15).
type strip struct {
	Tiers []stripTier
	Class string // "strip-lg" on the landing page, "strip-mini" in the admin table
	Links bool   // each bar links to its exercise
	Label string // the accessible name of the whole strip when the bars are not links
}

type stripTier struct {
	N     int
	Hours float64
	Bars  []stripBar
}

type stripBar struct {
	*course.Exercise
	State string // "", "started", "passed" or "here"
}

// Name is the bar's accessible name: "07 Retries with backoff, jitter and
// context, 2.5 h, started".
func (b stripBar) Name() string {
	state := map[string]string{"": "not started", "started": "started", "passed": "passed", "here": "shown above"}[b.State]
	return fmt.Sprintf("%02d %s, %s, %s", b.Number, b.Title, formatHours(b.Hours), state)
}

// courseStrip builds the strip; states maps a slug to "started", "passed"
// or "here".
func courseStrip(c *course.Course, states map[string]string) strip {
	var s strip
	for _, t := range c.Tiers {
		st := stripTier{N: t.N, Hours: t.Hours()}
		for _, e := range t.Exercises {
			st.Bars = append(st.Bars, stripBar{Exercise: e, State: states[e.Slug]})
		}
		s.Tiers = append(s.Tiers, st)
	}
	return s
}

// xSection is 1 section of an explanation: its heading and its HTML.
type xSection struct {
	Title string
	HTML  template.HTML
	Open  bool // "What it means" and "How to find it" start open
	Links int  // the number of links, for "Learn more"
}

// The catalog renders its "## " sections as <h3>, below the title.
var h2Split = regexp.MustCompile(`(?s)<h[23][^>]*>(.*?)</h[23]>`)

// explanationSections splits the catalog HTML at each heading, as the IDE
// does in the browser.
func explanationSections(html string) []xSection {
	locs := h2Split.FindAllStringSubmatchIndex(html, -1)
	var out []xSection
	for i, m := range locs {
		end := len(html)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		title := html[m[2]:m[3]]
		body := html[m[1]:end]
		sec := xSection{
			Title: title,
			// The catalog renders the HTML with raw HTML disabled and every
			// captured value escaped (internal/explain).
			HTML: template.HTML(body), //nolint:gosec // see above
			Open: title == "What it means" || title == "How to find it",
		}
		if title == "Learn more" {
			sec.Links = strings.Count(body, "<a ")
		}
		out = append(out, sec)
	}
	return out
}

// landingFigure is Figure 2 of the landing page: a compile error as the
// IDE explains it, from the real catalog entry.
type landingFigure struct {
	Title    template.HTML // the plain title, with the captured name in <code>
	Sections []xSection
	Course   string
}

func figureTwo() landingFigure {
	e := explain.Explain(runnerapi.Diagnostic{
		File: "retry.go", Line: 120, Column: 11, Severity: runnerapi.SeverityError,
		Source: runnerapi.SourceCompile, Message: "undefined: sleepContext",
	})
	var secs []xSection
	for _, s := range explanationSections(e.HTML) {
		if s.Title == "What it means" || s.Title == "How to find it" {
			secs = append(secs, s)
		}
	}
	return landingFigure{Title: codeCaptures(e.Title, e.Raw.Captures), Sections: secs, Course: e.Course}
}

// codeCaptures escapes a title and sets each captured value in <code>, as
// the IDE does (web/static/js/ide/explain.js, titleNodes).
func codeCaptures(title string, captures map[string]string) template.HTML {
	out := template.HTMLEscapeString(title)
	for k, v := range captures {
		if v == "" || k == "file" || k == "line" || k == "column" || k == "code" || k == "test" {
			continue
		}
		ev := template.HTMLEscapeString(v)
		out = strings.Replace(out, ev, "<code>"+ev+"</code>", 1)
	}
	return template.HTML(out) //nolint:gosec // every piece is escaped above
}

// guideView is the guide page: the root README's sections and the 4
// generic guides on reading Go messages.
type guideView struct {
	Sections []course.Section
	Guides   []explain.Explanation
}
