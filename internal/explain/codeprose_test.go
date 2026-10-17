package explain

import (
	"regexp"
	"strings"
	"testing"
)

// codeLike finds Go code in prose: a call such as Validate() or
// fmt.Errorf(...), or an index such as x[i]. The UI sets prose in B612,
// whose parentheses look like square brackets, so Validate() would read
// as Validate[]: code belongs in backticks, which the UI sets in mono.
var codeLike = regexp.MustCompile(`\b[A-Za-z_][\w.]*\([^()\s]*\)|\b[A-Za-z_]\w*\[[\w:.]*\]`)

var (
	fencedCode = regexp.MustCompile("(?s)```.*?```")
	inlineCode = regexp.MustCompile("`[^`]*`")
	linkTarget = regexp.MustCompile(`\]\([^)]*\)`)
)

// proseOnly removes code blocks, code spans and link targets from Markdown.
func proseOnly(md string) string {
	md = fencedCode.ReplaceAllString(md, "")
	md = inlineCode.ReplaceAllString(md, "")
	return linkTarget.ReplaceAllString(md, "]")
}

// Titles are plain text: the UI sets every captured value and every
// code-like word of a title in mono itself (titleNodes in
// web/static/js/ide/explain.js), so only the bodies need backticks.
func TestCodeIsMarkedAsCode(t *testing.T) {
	for _, e := range sortedEntries(catalog()) {
		for _, line := range strings.Split(proseOnly(e.body), "\n") {
			if m := codeLike.FindString(line); m != "" {
				t.Errorf("%s: %q is code outside backticks in: %s", e.path, m, strings.TrimSpace(line))
			}
		}
	}
}
