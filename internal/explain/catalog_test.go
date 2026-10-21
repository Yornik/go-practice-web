package explain

import (
	"cmp"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yornik/go-practice-web/internal/course"
)

// Word limits for the prose of a body (code blocks and headings do not
// count). catalog/README.md states them for authors.
const (
	minEntryWords, maxEntryWords = 120, 250
	minGuideWords, maxGuideWords = 150, 400
)

func TestLoaderRejects(t *testing.T) {
	valid := entryFile("compile-x", "compile", "match: ^x: (?P<name>\\S+)$\ntitle: {{name}} is x\n", "About `{{name}}`.")
	tests := []struct {
		name string
		path string // under catalog/, without .md
		src  string
		want string
	}{
		{"valid", "compile/compile-x", valid, ""},
		{"no front matter", "compile/compile-x", "## What it means\n", "front matter"},
		{"unknown key", "compile/compile-x", strings.Replace(valid, "title:", "tags: a\ntitle:", 1), `unknown key "tags"`},
		{"key twice", "compile/compile-x", strings.Replace(valid, "title:", "title: a\ntitle:", 1), "title appears twice"},
		{"bad regexp", "compile/compile-x", strings.Replace(valid, "(?P<name>\\S+)", "(?P<name>\\S+", 1), "missing closing )"},
		{"missing match", "compile/compile-x", strings.Replace(valid, "match: ^x: (?P<name>\\S+)$\n", "", 1), "needs match"},
		{"groups differ", "compile/compile-x", strings.Replace(valid, "title:", "match: ^y: (?P<other>\\S+)$\ntitle:", 1), "same named groups"},
		{"id is not the file name", "compile/compile-y", valid, `must equal the file name "compile-y"`},
		{"source is not the directory", "vet/compile-x", valid, `must equal the directory "vet"`},
		{"id without the source", "compile/x", strings.Replace(valid, "id: compile-x", "id: x", 1), "must start with the source"},
		{"unknown source", "magic/magic-x", strings.NewReplacer("compile-x", "magic-x", "source: compile", "source: magic").Replace(valid), `source "magic"`},
		{"bad course", "compile/compile-x", strings.Replace(valid, "title:", "course: workerpool\ntitle:", 1), "exercise slug"},
		{"no links", "compile/compile-x", strings.Replace(valid, "- Go | https://go.dev/doc/\n", "", 1), "at least 1 link"},
		{"http link", "compile/compile-x", strings.Replace(valid, "https://go.dev/doc/", "http://go.dev/doc/", 1), "https"},
		{"link host", "compile/compile-x", strings.Replace(valid, "https://go.dev/doc/", "https://example.com/go", 1), `host "example.com"`},
		{"link without label", "compile/compile-x", strings.Replace(valid, "- Go | ", "- ", 1), "label | url"},
		{"body link host", "compile/compile-x", strings.Replace(valid, "About", "[See](https://example.org/) About", 1), "in the body"},
		{"placeholder in a link target", "compile/compile-x", strings.Replace(valid, "About", "[See](https://go.dev/{{name}}) About", 1), "link target"},
		{"placeholder in an autolink", "compile/compile-x", strings.Replace(valid, "About", "<https://go.dev/{{name}}> About", 1), "link target"},
		{"unknown placeholder", "compile/compile-x", strings.Replace(valid, "About", "{{nope}} About", 1), "{{nope}} is not a group"},
		{"placeholder with spaces", "compile/compile-x", strings.Replace(valid, "About", "{{ name }} About", 1), "malformed placeholder"},
		{"unclosed placeholder", "compile/compile-x", strings.Replace(valid, "About", "{{name} About", 1), "malformed placeholder"},
		{"placeholder in a code block", "compile/compile-x", strings.Replace(valid, "## Example\n\nText.", "## Example\n\n```go\nx := {{name}}\n```", 1), "does not survive rendering"},
		{"missing section", "compile/compile-x", strings.Replace(valid, "## Example\n\nText.\n\n", "", 1), "sections must be"},
		{"sections out of order", "compile/compile-x", strings.Replace(strings.Replace(valid, "## Example", "## Swap", 1), "## How to find it", "## Example", 1), "sections must be"},
		{"empty section", "compile/compile-x", strings.Replace(valid, "## Example\n\nText.", "## Example\n", 1), `"Example" is empty`},
		{"text before the first section", "compile/compile-x", strings.Replace(valid, "---\n## What", "---\nIntro.\n\n## What", 1), "before the first section"},
		{"other heading level", "compile/compile-x", strings.Replace(valid, "## Example\n\nText.", "## Example\n\n### More\n\nText.", 1), "use ## for sections"},
		{"open code block", "compile/compile-x", strings.Replace(valid, "## Example\n\nText.", "## Example\n\n```go\nx := 1\n", 1), "not closed"},
		{"image", "compile/compile-x", strings.Replace(valid, "About", "![x](https://go.dev/x.png) About", 1), "images"},
		{"vet with code", "vet/vet-x", strings.NewReplacer("compile-x", "vet-x", "source: compile", "source: vet", "match:", "code: SA1000\nmatch:").Replace(valid), "analyzer, not code"},
		{"vet unknown analyzer", "vet/vet-x", strings.NewReplacer("compile-x", "vet-x", "source: compile", "source: vet", "match:", "analyzer: prinft\nmatch:").Replace(valid), `analyzer "prinft"`},
		{"staticcheck bad code", "staticcheck/staticcheck-x", strings.NewReplacer("compile-x", "staticcheck-x", "source: compile", "source: staticcheck", "match:", "code: SA40X\nmatch:").Replace(valid), `code "SA40X"`},
		{"staticcheck with analyzer", "staticcheck/staticcheck-x", strings.NewReplacer("compile-x", "staticcheck-x", "source: compile", "source: staticcheck", "match:", "analyzer: printf\nmatch:").Replace(valid), "code, not analyzer"},
		{"compile with code", "compile/compile-x", strings.Replace(valid, "match:", "code: SA1000\nmatch:", 1), "only staticcheck entries have code"},
		{"fallback", "compile/compile-x", strings.Replace(valid, "title:", "fallback: true\ntitle:", 1), ""},
		{"fallback not true", "compile/compile-x", strings.Replace(valid, "title:", "fallback: yes\ntitle:", 1), "must be true or left out"},
		{"fallback on vet", "vet/vet-x", strings.NewReplacer("compile-x", "vet-x", "source: compile", "source: vet", "match:", "analyzer: printf\nfallback: true\nmatch:").Replace(valid), "fallback is only for"},
		{"builtin of another source", "compile/compile-x", strings.Replace(valid, "About", "{{func}} About", 1), "{{func}} is not a group"},
		{"guide with match", "guide/guide-x", strings.Replace(guideFile("guide-x"), "title:", "match: x\ntitle:", 1), "only a fallback"},
		{"guide without Learn more", "guide/guide-x", strings.Replace(guideFile("guide-x"), "## Learn more", "## More\n\nText.", 1), "ends with"},
		{"guide with a placeholder", "guide/guide-x", strings.Replace(guideFile("guide-x"), "Text.", "{{file}}", 1), "{{file}} is not a group"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseEntry("catalog/"+tc.path+".md", []byte(tc.src))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && err == nil:
				t.Fatalf("no error, want one containing %q", tc.want)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestLoaderReportsCatalogProblems(t *testing.T) {
	fsys := fstest.MapFS{
		"catalog/compile/compile-x.md": {Data: []byte("not an entry")},
	}
	_, err := loadCatalog(fsys)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"compile-x.md: must start with a front matter", `the guide "guide-error-message" is missing`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q:\n%v", want, err)
		}
	}
	c := testCatalog(t, nil)
	if err := c.add(c.byID[GuideErrorMessage]); err == nil || !strings.Contains(err.Error(), "also used by") {
		t.Errorf("duplicate id: %v", err)
	}
}

// TestCatalogStyle checks the writing rules of catalog/README.md that a
// program can check.
func TestCatalogStyle(t *testing.T) {
	c := catalog()
	courseDir := filepath.Join(cmp.Or(os.Getenv("GO_PRACTICE_COURSE"), filepath.Join("..", "..", "course")), "exercises")
	_, err := os.Stat(courseDir)
	haveCourse := err == nil
	for _, e := range sortedEntries(c) {
		t.Run(e.id, func(t *testing.T) {
			words := proseWords(e.body)
			lo, hi := minEntryWords, maxEntryWords
			if e.source == SourceGuide {
				lo, hi = minGuideWords, maxGuideWords
			}
			if words < lo || words > hi {
				t.Errorf("%s: %d words of prose, want %d to %d", e.path, words, lo, hi)
			}
			title := e.title.fill(nil, func(string) string { return "x" })
			if strings.HasSuffix(title, ".") {
				t.Errorf("%s: the title ends with a period", e.path)
			}
			if e.source != SourceGuide && !strings.Contains(section(e.body, "Example"), "```") {
				t.Errorf("%s: the Example section has no code block", e.path)
			}
			if e.course != "" && haveCourse {
				// The catalog may name an exercise by its slug of the first
				// course; the site renames it (Explanation.WithCourse).
				_, err := os.Stat(filepath.Join(courseDir, e.course))
				if renamed, ok := course.Renamed(e.course); err != nil && ok {
					_, err = os.Stat(filepath.Join(courseDir, renamed))
				}
				if err != nil {
					t.Errorf("%s: course %q is not an exercise in course/exercises", e.path, e.course)
				}
			}
		})
	}
}

// TestEveryEntryHasFixture enforces docs/DESIGN.md: every entry is tested
// against the real message text.
func TestEveryEntryHasFixture(t *testing.T) {
	fixtures, err := readFixtures()
	if err != nil {
		t.Fatal(err)
	}
	c := catalog()
	covered := make(map[string]bool)
	for _, fx := range fixtures {
		covered[fx.id] = true
		if _, ok := c.byID[fx.id]; !ok {
			t.Errorf("fixture %s expects %q, which is not in the catalog", fx.dir, fx.id)
		}
	}
	for _, e := range sortedEntries(c) {
		if e.source != SourceGuide && !covered[e.id] {
			t.Errorf("%s has no fixture in testdata/fixtures (want file with id: %s)", e.path, e.id)
		}
	}
}

// TestLinks fetches every link of the catalog and checks that the page
// and its #fragment exist. It needs the network, so it runs only with
// EXPLAIN_CHECK_LINKS=1.
func TestLinks(t *testing.T) {
	if os.Getenv("EXPLAIN_CHECK_LINKS") == "" {
		t.Skip("set EXPLAIN_CHECK_LINKS=1 to fetch every link")
	}
	users := make(map[string][]string)
	for _, e := range sortedEntries(catalog()) {
		for _, l := range e.links {
			users[l.URL] = append(users[l.URL], e.id)
		}
		_, urls, _ := render(e.body)
		for _, u := range urls {
			users[u] = append(users[u], e.id)
		}
	}
	pages := make(map[string]string)
	var mu sync.Mutex
	client := &http.Client{Timeout: 30 * time.Second}
	fetch := func(t *testing.T, page string) string {
		mu.Lock()
		defer mu.Unlock()
		if body, ok := pages[page]; ok {
			return body
		}
		// pkg.go.dev answers 429 to a burst of requests; wait as long as it
		// asks, and a little longer each time, instead of failing the link.
		for attempt := 1; ; attempt++ {
			resp, err := client.Get(page)
			if err != nil {
				t.Fatalf("GET %s: %v", page, err)
			}
			b, err := io.ReadAll(resp.Body)
			if cerr := resp.Body.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				t.Fatalf("GET %s: %v", page, err)
			}
			if resp.StatusCode == http.StatusTooManyRequests && attempt < 8 {
				wait := time.Duration(attempt) * 2 * time.Second
				if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
					wait = max(wait, time.Duration(s)*time.Second)
				}
				time.Sleep(wait)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: %s", page, resp.Status)
			}
			pages[page] = string(b)
			return pages[page]
		}
	}
	var all []string
	for u := range users {
		all = append(all, u)
	}
	sort.Strings(all)
	for _, u := range all {
		t.Run(u, func(t *testing.T) {
			parsed, err := url.Parse(u)
			if err != nil {
				t.Fatal(err)
			}
			frag := parsed.Fragment
			parsed.Fragment = ""
			body := fetch(t, parsed.String())
			// Minified pages such as staticcheck.dev leave attributes unquoted.
			anchor := regexp.MustCompile(`\b(?:id|name)=["']?` + regexp.QuoteMeta(frag) + `(?:["'\s>]|$)`)
			if frag != "" && !anchor.MatchString(body) {
				t.Errorf("%s has no anchor %q (used by %v)", parsed, frag, users[u])
			}
		})
	}
}

// TestShow prints 1 entry as the UI gets it, with each placeholder filled
// in with its own name, so an author can read the result:
// EXPLAIN_SHOW=compile-undefined go test -run TestShow -v ./internal/explain/
func TestShow(t *testing.T) {
	id := os.Getenv("EXPLAIN_SHOW")
	if id == "" {
		t.Skip("set EXPLAIN_SHOW to an entry id")
	}
	e, ok := catalog().byID[id]
	if !ok {
		t.Fatalf("no entry %q", id)
	}
	values := make(map[string]string)
	for _, name := range append(slices.Clone(builtins[e.source]), e.groups...) {
		values[name] = "<" + name + ">"
	}
	x := e.explanation(values, Raw{Source: e.source}, e.source == SourceGuide)
	t.Logf("title: %s\ncourse: %s\nlinks: %v\n\n%s", x.Title, x.Course, x.Links, x.HTML)
}

func sortedEntries(c *catalogData) []*entry {
	var es []*entry
	for _, e := range c.byID {
		es = append(es, e)
	}
	slices.SortFunc(es, func(a, b *entry) int { return strings.Compare(a.path, b.path) })
	return es
}

var codeBlockRE = regexp.MustCompile("(?s)(```|~~~).*?(```|~~~)")

// proseWords counts the words outside code blocks and headings.
func proseWords(body string) int {
	n := 0
	for _, line := range strings.Split(codeBlockRE.ReplaceAllString(body, ""), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		n += len(strings.Fields(line))
	}
	return n
}

// section returns the Markdown of 1 "## " section.
func section(body, title string) string {
	_, rest, ok := strings.Cut(body, "## "+title+"\n")
	if !ok {
		return ""
	}
	if i := strings.Index(rest, "\n## "); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
