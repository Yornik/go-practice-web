package course

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// GitHubBlobURL is the prefix for links to course files that the site does
// not show itself.
const GitHubBlobURL = "https://github.com/Yornik/go-practice/blob/main/"

// Section is a part of a rendered Markdown document that starts at a "## "
// heading. HTML holds the body without the heading itself, so the UI can
// put the title where it fits (a tab, a card header).
type Section struct {
	ID    string        // the heading's generated id, for example "run-the-tests"
	Title string        // the heading text, for example "Run the tests"
	HTML  template.HTML // the body below the heading
}

// docPathKey carries the repository-relative path of the document being
// parsed, so the link rewriter can resolve relative links against it.
var docPathKey = parser.NewContextKey()

// markdown renders the course's Markdown. It is built once per Load because
// the link rewriter needs the set of exercise slugs.
type markdown struct {
	md goldmark.Markdown
}

func newMarkdown(slugs map[string]bool) *markdown {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				// Classes instead of inline styles: the site switches between a
				// light and a dark theme with CSS (see HighlightCSS), and the CSP
				// stays free of per-element style attributes.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(&linkRewriter{slugs: slugs}, 100)),
		),
		// Raw HTML stays disabled (no html.WithUnsafe). Instead of dropping it
		// as goldmark does, literalHTML shows it as text: in the course a
		// "<Field>" outside backticks is a placeholder, not markup.
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(literalHTML{}, 100))),
	)
	return &markdown{md: md}
}

// document is a parsed Markdown file.
type document struct {
	src  []byte
	root ast.Node
}

func (m *markdown) parse(src []byte, docPath string) document {
	pc := parser.NewContext()
	pc.Set(docPathKey, docPath)
	root := m.md.Parser().Parse(text.NewReader(src), parser.WithContext(pc))
	return document{src: src, root: root}
}

// renderNodes renders a list of sibling block nodes.
func (m *markdown) renderNodes(d document, nodes []ast.Node) (template.HTML, error) {
	var buf bytes.Buffer
	for _, n := range nodes {
		if err := m.md.Renderer().Render(&buf, d.src, n); err != nil {
			return "", fmt.Errorf("render markdown: %w", err)
		}
	}
	return template.HTML(buf.String()), nil //nolint:gosec // goldmark output with raw HTML disabled
}

// render renders a whole Markdown file.
func (m *markdown) render(src []byte, docPath string) (template.HTML, error) {
	d := m.parse(src, docPath)
	return m.renderNodes(d, []ast.Node{d.root})
}

// renderInline renders a single line of Markdown without the surrounding
// paragraph, for text that the UI places inside its own element.
func (m *markdown) renderInline(src string) (template.HTML, error) {
	h, err := m.render([]byte(src), "")
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(h))
	s = strings.TrimPrefix(s, "<p>")
	s = strings.TrimSuffix(s, "</p>")
	return template.HTML(s), nil //nolint:gosec // see renderNodes
}

// splitDoc is a document cut at its "## " headings.
type splitDoc struct {
	preamble []ast.Node // the blocks before the first "## " heading
	sections []rawSection
}

type rawSection struct {
	id, title string
	heading   *ast.Heading
	body      []ast.Node
}

func split(d document) splitDoc {
	var s splitDoc
	for n := d.root.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 2 {
			id, _ := h.AttributeString("id")
			idStr, _ := id.([]byte)
			s.sections = append(s.sections, rawSection{
				id:      string(idStr),
				title:   plainText(h, d.src),
				heading: h,
			})
			continue
		}
		if len(s.sections) == 0 {
			s.preamble = append(s.preamble, n)
			continue
		}
		last := &s.sections[len(s.sections)-1]
		last.body = append(last.body, n)
	}
	return s
}

// sectionSource returns the Markdown source of a section's body, from the
// line after its heading to the next "## " heading. The step parser reads
// the "Run the tests" commands from it.
func sectionSource(d document, s rawSection) string {
	start := 0
	if lines := s.heading.Lines(); lines.Len() > 0 {
		start = lines.At(lines.Len() - 1).Stop
	}
	end := len(d.src)
	for n := s.heading.NextSibling(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 2 {
			if lines := h.Lines(); lines.Len() > 0 {
				end = lines.At(0).Start
			}
			break
		}
	}
	if start > end {
		return ""
	}
	return string(d.src[start:end])
}

// renderSplit renders a split document: the whole document, and each
// section's body on its own.
func (m *markdown) renderSplit(d document, s splitDoc) (template.HTML, []Section, error) {
	full, err := m.renderNodes(d, s.preamble)
	if err != nil {
		return "", nil, err
	}
	out := make([]Section, 0, len(s.sections))
	for _, r := range s.sections {
		heading, err := m.renderNodes(d, []ast.Node{r.heading})
		if err != nil {
			return "", nil, err
		}
		body, err := m.renderNodes(d, r.body)
		if err != nil {
			return "", nil, fmt.Errorf("section %q: %w", r.title, err)
		}
		full += heading + body
		out = append(out, Section{ID: r.id, Title: r.title, HTML: body})
	}
	return full, out, nil
}

// plainText returns the text of an inline tree, without markup.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// linkRewriter points relative links in the course at the site: a link to
// an exercise opens its page, any other file opens on GitHub, because the
// site only serves exercises.
type linkRewriter struct {
	slugs map[string]bool
}

var exerciseLink = regexp.MustCompile(`^exercises/([^/]+)(?:/README\.md)?$`)

// Transform implements parser.ASTTransformer.
func (r *linkRewriter) Transform(doc *ast.Document, _ text.Reader, pc parser.Context) {
	base, _ := pc.Get(docPathKey).(string)
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch l := n.(type) {
		case *ast.Link:
			l.Destination = []byte(r.rewrite(base, string(l.Destination), false))
		case *ast.Image:
			l.Destination = []byte(r.rewrite(base, string(l.Destination), true))
		}
		return ast.WalkContinue, nil
	})
}

func (r *linkRewriter) rewrite(base, dest string, image bool) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		// Absolute URLs, mailto:, in-page anchors and unparsable links stay.
		return dest
	}
	var p string
	if strings.HasPrefix(u.Path, "/") {
		// GitHub resolves a leading slash against the repository root.
		p = path.Clean(strings.TrimPrefix(u.Path, "/"))
	} else {
		p = path.Join(path.Dir(base), u.Path)
	}
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		p = ""
	}
	frag := ""
	if u.Fragment != "" {
		frag = "#" + u.EscapedFragment()
	}
	if m := exerciseLink.FindStringSubmatch(p); m != nil && r.slugs[m[1]] && !image {
		return "/learn/" + m[1] + frag
	}
	if image {
		// The raw file, so the browser can show it.
		return "https://github.com/Yornik/go-practice/raw/main/" + p
	}
	return GitHubBlobURL + p + frag
}

// Default highlighting themes for HighlightCSS.
const (
	DefaultLightStyle = "github"
	DefaultDarkStyle  = "github-dark"
)

// HighlightCSS returns the CSS for highlighted code with the default themes.
// See HighlightCSSFor.
func HighlightCSS() string {
	css, err := HighlightCSSFor(DefaultLightStyle, DefaultDarkStyle)
	if err != nil {
		// The default style names are constants that chroma ships.
		panic(err)
	}
	return css
}

// HighlightCSSFor returns the CSS for code blocks highlighted with classes,
// for a light and a dark chroma style. The light rules apply under
// [data-theme="light"], the dark rules under [data-theme="dark"]. Without a
// data-theme attribute the system preference decides, so a page renders
// correctly before its script runs.
func HighlightCSSFor(lightStyle, darkStyle string) (string, error) {
	light, err := styleCSS(lightStyle)
	if err != nil {
		return "", err
	}
	dark, err := styleCSS(darkStyle)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "/* Code highlighting: %s (light) and %s (dark), generated by chroma. */\n", lightStyle, darkStyle)
	writeScoped(&b, light, `[data-theme="light"]`, `:root:not([data-theme])`)
	writeScoped(&b, dark, `[data-theme="dark"]`)
	b.WriteString("@media (prefers-color-scheme: dark) {\n")
	writeScoped(&b, dark, `:root:not([data-theme])`)
	b.WriteString("}\n")
	return b.String(), nil
}

type cssRule struct{ selector, body string }

func styleCSS(name string) ([]cssRule, error) {
	style, ok := styles.Registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown chroma style %q", name)
	}
	var buf bytes.Buffer
	f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithCSSComments(false))
	if err := f.WriteCSS(&buf, style); err != nil {
		return nil, fmt.Errorf("chroma css for %q: %w", name, err)
	}
	var rules []cssRule
	for line := range strings.Lines(buf.String()) {
		sel, body, ok := strings.Cut(strings.TrimSpace(line), " { ")
		if !ok {
			continue
		}
		rules = append(rules, cssRule{selector: sel, body: strings.TrimSuffix(body, " }")})
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("chroma css for %q: no rules", name)
	}
	return rules, nil
}

func writeScoped(b *strings.Builder, rules []cssRule, scopes ...string) {
	for _, r := range rules {
		sels := make([]string, len(scopes))
		for i, s := range scopes {
			sels[i] = s + " " + r.selector
		}
		fmt.Fprintf(b, "%s { %s }\n", strings.Join(sels, ", "), r.body)
	}
}

// literalHTML renders raw HTML from the Markdown source as escaped text.
type literalHTML struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (literalHTML) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			segs := n.(*ast.RawHTML).Segments
			for i := range segs.Len() {
				seg := segs.At(i)
				_, _ = w.WriteString(template.HTMLEscapeString(string(seg.Value(src))))
			}
		}
		return ast.WalkSkipChildren, nil
	})
	reg.Register(ast.KindHTMLBlock, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		b := n.(*ast.HTMLBlock)
		_, _ = w.WriteString("<pre><code>")
		for i := range b.Lines().Len() {
			line := b.Lines().At(i)
			_, _ = w.WriteString(template.HTMLEscapeString(string(line.Value(src))))
		}
		if b.HasClosure() {
			_, _ = w.WriteString(template.HTMLEscapeString(string(b.ClosureLine.Value(src))))
		}
		_, _ = w.WriteString("</code></pre>\n")
		return ast.WalkSkipChildren, nil
	})
}
