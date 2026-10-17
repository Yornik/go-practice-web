package explain

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// The README in catalog/ is for authors; only catalog/<source>/*.md files
// are entries.
//
//go:embed catalog/*/*.md
var catalogFS embed.FS

// Sections of an entry body, in this order. Guides choose their own
// sections but end with "Learn more".
var entrySections = []string{"What it means", "Why Go does this", "How to find it", "Example", "Learn more"}

const learnMore = "Learn more"

// builtins lists the values the engine fills in for each source, next to
// an entry's regexp groups. catalog/README.md documents them.
var builtins = map[string][]string{
	SourceCompile:     {"file", "line", "column", "code"},
	SourceVet:         {"file", "line", "column", "code"},
	SourceStaticcheck: {"file", "line", "column", "code"},
	SourceBuild:       {"file", "line", "column", "code"},
	SourceRuntime:     {"func", "file", "line", "test"},
	SourceRace:        {"func", "file", "line", "access", "prev_func", "prev_file", "prev_line", "prev_access", "test"},
	SourceTest:        {"file", "line", "test"},
	SourceGuide:       nil,
}

// vetAnalyzers are the analyzers of go vet in the runner's Go release
// (go tool vet help). A typo in an entry's analyzer would never match, so
// the loader rejects unknown names; add new analyzers when Go adds them.
var vetAnalyzers = []string{
	"appends", "asmdecl", "assign", "atomic", "bools", "buildtag", "cgocall",
	"composites", "copylocks", "defers", "directive", "errorsas", "framepointer",
	"hostport", "httpresponse", "ifaceassert", "loopclosure", "lostcancel",
	"nilfunc", "printf", "shift", "sigchanyzer", "slog", "stdmethods",
	"stdversion", "stringintconv", "structtag", "testinggoroutine", "tests",
	"timeformat", "unmarshal", "unreachable", "unsafeptr", "unusedresult",
	"waitgroup",
}

// linkHosts are the sources a learner can trust and that the course rules
// allow: the Go project's own sites and staticcheck's documentation.
var linkHosts = []string{"go.dev", "pkg.go.dev", "staticcheck.dev"}

var (
	idRE           = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	courseRE       = regexp.MustCompile(`^[0-9]{2}-[a-z0-9]+(?:-[a-z0-9]+)*$`)
	staticcheckRE  = regexp.MustCompile(`^[A-Z]+[0-9](?:[0-9]{3})?$`)
	keyRE          = regexp.MustCompile(`^[a-z]+$`)
	placeholderRE  = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)
	brokenPlaceRE  = regexp.MustCompile(`\{\{\s+[A-Za-z_]\w*\s*\}\}|\{\{[A-Za-z_]\w*\s+\}\}|\{\{[A-Za-z_]\w*\}(?:[^}]|$)`)
	headingRE      = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	fenceRE        = regexp.MustCompile("^\\s{0,3}(```|~~~)")
	trailingCodeRE = regexp.MustCompile(`^(?s)(.*) \(([A-Z]+[0-9]+|compile)\)$`)
	markdownLinkRE = regexp.MustCompile(`\]\(\s*<?([^)\s>]*)`)
	categoryRE     = regexp.MustCompile(`^[A-Z]+[0-9]$`) // for example "SA4"
)

// entry is one parsed catalog file.
type entry struct {
	id       string
	source   string
	path     string
	patterns []*regexp.Regexp
	groups   []string // the named groups, the same set in every pattern
	// code is the vet analyzer or the staticcheck code; category is true
	// for a staticcheck category such as "SA4", which matches by prefix.
	code     string
	category bool
	// fallback marks a broad pattern, such as any got/want message: it
	// loses to every other pattern that matches the same message.
	fallback bool
	title    tmpl
	html     tmpl
	links    []Link
	course   string
	body     string // Markdown, for the style checks in the tests
}

// catalogData is the loaded catalog. It is immutable after loading, so the
// explain functions are safe for concurrent use.
type catalogData struct {
	byID     map[string]*entry
	bySource map[string][]*entry
}

var (
	loadOnce sync.Once
	loaded   *catalogData
	loadErr  error
)

func loadDefault() (*catalogData, error) {
	loadOnce.Do(func() { loaded, loadErr = loadCatalog(catalogFS) })
	return loaded, loadErr
}

// Load the catalog when the package initialises, so the first request does
// not pay for rendering it.
func init() { _, _ = loadDefault() }

func catalog() *catalogData {
	c, _ := loadDefault()
	return c
}

// loadCatalog reads every catalog/<source>/<id>.md file of fsys. It keeps
// the valid entries and returns the problems of the others as 1 error.
func loadCatalog(fsys fs.FS) (*catalogData, error) {
	c := &catalogData{byID: make(map[string]*entry), bySource: make(map[string][]*entry)}
	paths, err := fs.Glob(fsys, "catalog/*/*.md")
	if err != nil {
		return c, err
	}
	sort.Strings(paths)
	var errs []error
	for _, p := range paths {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		e, err := parseEntry(p, data)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := c.add(e); err != nil {
			errs = append(errs, err)
		}
	}
	for _, id := range []string{GuideErrorMessage, GuideStackTrace, GuideRaceReport, GuideFailingTest} {
		if e, ok := c.byID[id]; !ok || e.source != SourceGuide {
			errs = append(errs, fmt.Errorf("catalog: the guide %q is missing", id))
		}
	}
	return c, errors.Join(errs...)
}

// add adds an entry unless its id is taken. The file layout already makes
// ids unique (the file name is the id), so this guards against a change of
// that rule.
func (c *catalogData) add(e *entry) error {
	if prev, dup := c.byID[e.id]; dup {
		return fmt.Errorf("%s: id %q is also used by %s", e.path, e.id, prev.path)
	}
	c.byID[e.id] = e
	c.bySource[e.source] = append(c.bySource[e.source], e)
	return nil
}

// parseEntry parses and validates one catalog file. It reports every
// problem it finds, so an author can fix them in 1 pass.
func parseEntry(p string, data []byte) (*entry, error) {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: "+format, append([]any{p}, args...)...))
	}
	src := normalizeNewlines(string(data))
	if !utf8.ValidString(src) {
		fail("not valid UTF-8")
		return nil, errors.Join(errs...)
	}
	front, body, ok := splitFrontMatter(src)
	if !ok {
		fail("must start with a front matter block between 2 lines of ---")
		return nil, errors.Join(errs...)
	}
	fields, err := parseFrontMatter(front)
	if err != nil {
		fail("%v", err)
		return nil, errors.Join(errs...)
	}
	e := &entry{path: p}
	e.id = fields.one("id")
	e.source = fields.one("source")
	e.course = fields.one("course")

	// Identity: the file name is the id and the directory is the source, so
	// an entry is easy to find from an explanation in the UI.
	dir, file := path.Base(path.Dir(p)), strings.TrimSuffix(path.Base(p), ".md")
	switch {
	case e.id == "":
		fail("id is missing")
	case !idRE.MatchString(e.id):
		fail("id %q must be lower-case words joined by -", e.id)
	case e.id != file:
		fail("id %q must equal the file name %q", e.id, file)
	}
	if _, known := builtins[e.source]; !known {
		fail("source %q is not one of compile, vet, staticcheck, runtime, race, test, build, guide", e.source)
		return nil, errors.Join(errs...)
	}
	if dir != e.source {
		fail("source %q must equal the directory %q", e.source, dir)
	}
	if !strings.HasPrefix(e.id, e.source+"-") {
		fail("id %q must start with the source and a dash: %q", e.id, e.source+"-")
	}
	if e.course != "" && !courseRE.MatchString(e.course) {
		fail("course %q must be an exercise slug such as 12-workerpool", e.course)
	}

	// What the entry matches on.
	for _, m := range fields.all("match") {
		re, err := regexp.Compile(m)
		if err != nil {
			fail("match %q: %v", m, err)
			continue
		}
		e.patterns = append(e.patterns, re)
	}
	analyzer, code := fields.one("analyzer"), fields.one("code")
	hasMatch := len(fields.all("match")) > 0
	switch e.source {
	case SourceGuide:
		if hasMatch || analyzer != "" || code != "" {
			fail("a guide has no match, analyzer or code; it is only a fallback")
		}
	case SourceVet:
		if code != "" {
			fail("a vet entry names its analyzer with analyzer, not code")
		}
		if analyzer == "" && !hasMatch {
			fail("a vet entry needs analyzer, match or both")
		}
		if analyzer != "" && !slices.Contains(vetAnalyzers, analyzer) {
			fail("analyzer %q is not a go vet analyzer (see go tool vet help)", analyzer)
		}
		e.code = analyzer
	case SourceStaticcheck:
		if analyzer != "" {
			fail("a staticcheck entry names its check with code, not analyzer")
		}
		if code == "" && !hasMatch {
			fail("a staticcheck entry needs code, match or both")
		}
		if code != "" && !staticcheckRE.MatchString(code) {
			fail("code %q must be a check such as SA4006 or a category such as SA4", code)
		}
		e.code = code
		e.category = categoryRE.MatchString(code)
	default:
		if analyzer != "" || code != "" {
			fail("only vet entries have analyzer and only staticcheck entries have code")
		}
		if !hasMatch {
			fail("a %s entry needs match", e.source)
		}
	}
	switch v := fields.one("fallback"); {
	case v == "":
	case v != "true":
		fail("fallback %q must be true or left out", v)
	case e.source == SourceVet || e.source == SourceStaticcheck || e.source == SourceGuide:
		// vet and staticcheck rank by analyzer and code instead.
		fail("fallback is only for compile, build, runtime, race and test entries")
	default:
		e.fallback = true
	}
	e.groups = groupNames(e.patterns, fail)

	// Links.
	for _, l := range fields.all("links") {
		label, u, ok := strings.Cut(l, " | ")
		label, u = strings.TrimSpace(label), strings.TrimSpace(u)
		if !ok || label == "" || u == "" {
			fail("link %q must be written as: label | url", l)
			continue
		}
		if err := checkURL(u); err != nil {
			fail("link %q: %v", u, err)
			continue
		}
		e.links = append(e.links, Link{Label: label, URL: u})
	}
	if len(e.links) == 0 {
		fail("links needs at least 1 link for Learn more")
	}

	// Placeholders: every name must be a regexp group or a built-in of the
	// source, and none may be malformed.
	allowed := append(slices.Clone(builtins[e.source]), e.groups...)
	title := fields.one("title")
	if title == "" {
		fail("title is missing")
	}
	if utf8.RuneCountInString(title) > 100 {
		fail("title is longer than 100 characters")
	}
	for _, part := range []struct{ name, text string }{{"title", title}, {"body", body}} {
		if m := brokenPlaceRE.FindString(part.text); m != "" {
			fail("%s: malformed placeholder %q; write {{name}} without spaces", part.name, m)
		}
		for _, m := range placeholderRE.FindAllStringSubmatch(part.text, -1) {
			if !slices.Contains(allowed, m[1]) {
				fail("%s: placeholder {{%s}} is not a group of match or a built-in of %s (%s)",
					part.name, m[1], e.source, strings.Join(allowed, ", "))
			}
		}
	}

	// Body.
	if err := checkSections(e.source, body); err != nil {
		fail("%v", err)
	}
	for _, m := range markdownLinkRE.FindAllStringSubmatch(body, -1) {
		if strings.Contains(m[1], "{{") {
			fail("a link target must not contain a placeholder: %q", m[1])
		}
	}
	rendered, urls, err := render(body)
	if err != nil {
		fail("render: %v", err)
	}
	for _, u := range urls {
		if strings.Contains(u, "{{") {
			fail("a link target must not contain a placeholder: %q", u)
		} else if err := checkURL(u); err != nil {
			fail("link %q in the body: %v", u, err)
		}
	}
	// Syntax highlighting splits a placeholder inside a code block into
	// tokens, so it would never be replaced. Catch that here.
	for _, name := range placeholderNames(body) {
		want := strings.Count(body, "{{"+name+"}}")
		if got := strings.Count(rendered, "{{"+name+"}}"); got != want {
			fail("placeholder {{%s}} does not survive rendering (%d of %d); placeholders cannot go in code blocks", name, got, want)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	e.body = body
	e.title = compileTmpl(title)
	e.html = compileTmpl(rendered + linksHTML(e.links, e.course))
	return e, nil
}

// fields holds the parsed front matter. Only match may repeat, and links is
// a list.
type fields map[string][]string

func (f fields) one(key string) string {
	if v := f[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func (f fields) all(key string) []string { return f[key] }

var frontKeys = map[string]bool{
	"id": true, "source": true, "match": true, "analyzer": true, "code": true,
	"title": true, "course": true, "links": true, "fallback": true,
}

func splitFrontMatter(src string) (front, body string, ok bool) {
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return "", "", false
	}
	front, body, ok = strings.Cut(rest, "\n---\n")
	if !ok {
		return "", "", false
	}
	return front, body, true
}

// parseFrontMatter reads "key: value" lines and, after "links:", list
// lines "- label | url". Values are taken literally: no quotes, no escapes,
// no comments, so a regexp or a URL with a # needs nothing special.
func parseFrontMatter(front string) (fields, error) {
	f := make(fields)
	list := ""
	for n, line := range strings.Split(front, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if item, ok := strings.CutPrefix(trimmed, "- "); ok {
			if list == "" {
				return nil, fmt.Errorf("front matter line %d: a list item outside links", n+2)
			}
			f[list] = append(f[list], strings.TrimSpace(item))
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || !keyRE.MatchString(key) {
			return nil, fmt.Errorf("front matter line %d: want key: value, got %q", n+2, line)
		}
		if !frontKeys[key] {
			return nil, fmt.Errorf("front matter line %d: unknown key %q", n+2, key)
		}
		list = ""
		if key == "links" {
			if value != "" {
				return nil, fmt.Errorf("front matter line %d: put each link on its own line: - label | url", n+2)
			}
			list = key
			continue
		}
		if _, dup := f[key]; dup && key != "match" {
			return nil, fmt.Errorf("front matter line %d: %s appears twice", n+2, key)
		}
		if value == "" {
			return nil, fmt.Errorf("front matter line %d: %s has no value", n+2, key)
		}
		f[key] = append(f[key], value)
	}
	return f, nil
}

// groupNames returns the named groups of the patterns. Every pattern must
// have the same set, so a placeholder has a value whichever pattern matched.
func groupNames(patterns []*regexp.Regexp, fail func(string, ...any)) []string {
	var first []string
	for i, re := range patterns {
		var names []string
		for _, n := range re.SubexpNames() {
			if n != "" && !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		if i == 0 {
			first = names
		} else if !slices.Equal(first, names) {
			fail("every match must have the same named groups: %v and %v", first, names)
		}
	}
	return first
}

func checkURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("must be an https URL")
	}
	if !slices.Contains(linkHosts, parsed.Host) {
		return fmt.Errorf("host %q is not one of %s", parsed.Host, strings.Join(linkHosts, ", "))
	}
	return nil
}

// checkSections checks the "## " sections of a body: the 5 fixed ones in
// order for an entry, at least 2 ending with "Learn more" for a guide. Only
// Learn more may be empty, because the links are added after it.
func checkSections(source, body string) error {
	type section struct {
		title string
		words int
	}
	var sections []section
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceRE.MatchString(line) {
			inFence = !inFence
		}
		if inFence {
			if len(sections) > 0 {
				sections[len(sections)-1].words++
			}
			continue
		}
		if m := headingRE.FindStringSubmatch(line); m != nil {
			if m[1] != "##" {
				return fmt.Errorf("heading %q: use ## for sections and no other heading levels", line)
			}
			sections = append(sections, section{title: m[2]})
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(sections) == 0 {
			return fmt.Errorf("text before the first section: %q", line)
		}
		sections[len(sections)-1].words += len(strings.Fields(line))
	}
	if inFence {
		return errors.New("a code block is not closed")
	}
	var titles []string
	for _, s := range sections {
		titles = append(titles, s.title)
		if s.words == 0 && s.title != learnMore {
			return fmt.Errorf("section %q is empty", s.title)
		}
	}
	if source == SourceGuide {
		if len(titles) < 2 || titles[len(titles)-1] != learnMore {
			return fmt.Errorf("a guide needs at least 2 sections and ends with %q; got %q", learnMore, titles)
		}
		return nil
	}
	if !slices.Equal(titles, entrySections) {
		return fmt.Errorf("sections must be %q in this order; got %q", entrySections, titles)
	}
	return nil
}

func placeholderNames(s string) []string {
	var names []string
	for _, m := range placeholderRE.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	return names
}

// md renders entry bodies like the course READMEs (internal/course):
// CommonMark, chroma classes for code, no raw HTML.
var md = goldmark.New(
	goldmark.WithExtensions(
		highlighting.NewHighlighting(highlighting.WithFormatOptions(chromahtml.WithClasses(true))),
	),
)

// render turns a body into HTML. Sections become h3, because the UI shows
// an explanation under its own heading. Links open in a new tab, because a
// learner reading an explanation is in the middle of editing. It returns
// the link targets so the caller can check them.
func render(body string) (string, []string, error) {
	src := []byte(body)
	doc := md.Parser().Parse(text.NewReader(src))
	var urls []string
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			n.Level++
		case *ast.Link:
			urls = append(urls, string(n.Destination))
			n.SetAttributeString("target", []byte("_blank"))
			n.SetAttributeString("rel", []byte("noopener noreferrer"))
		case *ast.AutoLink:
			urls = append(urls, string(n.URL(src)))
		case *ast.Image:
			return ast.WalkStop, errors.New("images are not allowed")
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", nil, err
	}
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return "", nil, err
	}
	return buf.String(), urls, nil
}

// linksHTML renders the front matter links and the course exercise as the
// list at the end of the Learn more section.
func linksHTML(links []Link, course string) string {
	var b strings.Builder
	b.WriteString(`<ul class="explain-links">`)
	for _, l := range links {
		fmt.Fprintf(&b, `<li><a href="%s" target="_blank" rel="noopener noreferrer">%s</a></li>`,
			html.EscapeString(l.URL), html.EscapeString(l.Label))
	}
	if course != "" {
		fmt.Fprintf(&b, `<li><a href="%s" target="_blank" rel="noopener">Course exercise %s</a></li>`,
			html.EscapeString(CourseURLPrefix+course), html.EscapeString(course))
	}
	b.WriteString("</ul>\n")
	return b.String()
}

// tmpl is a text with {{name}} placeholders, split once at load time so
// that filling it in never scans a captured value for placeholders.
type tmpl []segment

type segment struct {
	lit  string
	name string // set for a placeholder
}

func compileTmpl(s string) tmpl {
	var t tmpl
	last := 0
	for _, m := range placeholderRE.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			t = append(t, segment{lit: s[last:m[0]]})
		}
		t = append(t, segment{name: s[m[2]:m[3]]})
		last = m[1]
	}
	if last < len(s) {
		t = append(t, segment{lit: s[last:]})
	}
	return t
}

func (t tmpl) fill(values map[string]string, convert func(string) string) string {
	var b strings.Builder
	for _, s := range t {
		if s.name == "" {
			b.WriteString(s.lit)
			continue
		}
		b.WriteString(convert(values[s.name]))
	}
	return b.String()
}

// titleValue keeps a title on 1 short line whatever the message held.
func titleValue(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	const maxRunes = 60
	if utf8.RuneCountInString(v) > maxRunes {
		r := []rune(v)
		v = string(r[:maxRunes-1]) + "…"
	}
	return v
}
