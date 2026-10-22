package web

import (
	"html/template"
	"regexp"
	"strings"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/explain"
	"github.com/yornik/go-practice-web/internal/runnerapi"
)

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

// landingAnatomy is the anatomy drawing of the landing page (VESPER.md
// 6.30): a compile error in exercise 07, as the IDE takes it apart. The
// title and the meaning come from the real catalog entry, so the page
// promises nothing the IDE does not say.
type landingAnatomy struct {
	Title template.HTML // the plain title, with the captured name in <code>
	Means template.HTML // the first 2 sentences of "What it means"
}

// anatomyMessage is the drawn message. Its length is fixed: the leaders of
// the drawing are measured in characters of this line.
const anatomyMessage = "undefined: strings"

func newLandingAnatomy() landingAnatomy {
	e := explain.Explain(runnerapi.Diagnostic{
		File: "rollcall.go", Line: 36, Column: 12, Severity: runnerapi.SeverityError,
		Source: runnerapi.SourceCompile, Message: anatomyMessage,
	})
	a := landingAnatomy{Title: codeCaptures(e.Title, e.Raw.Captures)}
	for _, s := range explanationSections(e.HTML) {
		if s.Title == "What it means" {
			a.Means = firstSentences(s.HTML, 2)
		}
	}
	return a
}

// voidTag is an element without an end tag.
var voidTag = regexp.MustCompile(`^<(br|hr|img|wbr)\b`)

// paragraph is the first paragraph of catalog HTML.
var paragraph = regexp.MustCompile(`(?s)<p>(.*?)</p>`)

// firstSentences is the start of the first paragraph of catalog HTML, up
// to the end of its nth sentence: a title-block cell holds 2 sentences, not
// the whole section. A sentence ends at ". " outside every element, so the
// cut never splits a tag or leaves one open: "a. b" in <code> is no end.
func firstSentences(html template.HTML, n int) template.HTML {
	m := paragraph.FindStringSubmatch(string(html))
	if m == nil {
		return ""
	}
	p := m[1]
	depth := 0
	for i := 0; i < len(p); i++ {
		switch {
		case p[i] == '<':
			end := strings.IndexByte(p[i:], '>')
			if end < 0 {
				return ""
			}
			tag := p[i : i+end+1]
			switch {
			case strings.HasPrefix(tag, "</"):
				depth--
			case !strings.HasSuffix(tag, "/>") && !voidTag.MatchString(tag):
				depth++
			}
			i += end
		case depth == 0 && p[i] == '.' && i+1 < len(p) && p[i+1] == ' ':
			if n--; n == 0 {
				p = p[:i+1]
			}
		}
		if n == 0 {
			break
		}
	}
	// The catalog renders the HTML with every captured value escaped
	// (internal/explain), and the cut falls outside every element.
	return template.HTML(p) //nolint:gosec // see above
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

// guideView is the guide page (VESPER.md 8.2): a manual of numbered
// sections, the root README's among them, and the 4 generic guides on
// reading Go messages.
type guideView struct {
	Items  []guideItem
	Guides []explain.Explanation
}

// guideItem is 1 numbered section of the guide.
type guideItem struct {
	N     int
	ID    string
	Title string // the title in the contents and over the section
	// Kind picks the section's body: "story", "work", "checklist",
	// "messages", "keys" and "tools" have their own; "section" is the
	// README's HTML.
	Kind    string
	Section course.Section
}
