package course

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Level is a step of the hint ladder. Levels are revealed in order.
type Level int

// The hint ladder, in the order the course reveals it.
const (
	Hint1 Level = iota + 1
	Hint2
	Hint3
	Explanation
	Walkthrough
	Solution
)

// MaxLevel is the last level of the ladder.
const MaxLevel = Solution

// ErrLevel is returned for a level outside 1 to MaxLevel.
var ErrLevel = errors.New("course: no such hint level")

// Valid reports whether l is a level of the ladder.
func (l Level) Valid() bool { return l >= Hint1 && l <= MaxLevel }

// IsHint reports whether l is 1 of the 3 hints.
func (l Level) IsHint() bool { return l >= Hint1 && l <= Hint3 }

// String returns the name the course uses, for example "Hint 2".
func (l Level) String() string {
	switch l {
	case Hint1, Hint2, Hint3:
		return fmt.Sprintf("Hint %d", int(l))
	case Explanation:
		return "Explanation"
	case Walkthrough:
		return "Walkthrough"
	case Solution:
		return "Solution"
	}
	return fmt.Sprintf("Level(%d)", int(l))
}

// hidden is an exercise's hidden material. Load decodes and checks it; the
// HTML is rendered on the first reveal of each level, because highlighting
// it all would triple the start time for pages that few learners open. It
// stays unexported so that a template cannot reach it by accident: the
// reveal API hands it out through Exercise.Hidden.
type hidden struct {
	html     [MaxLevel + 1]func() (template.HTML, error) // index 0 unused
	solution []File
}

func (h *hidden) render(l Level) (template.HTML, error) {
	return h.html[l]()
}

// rot13 decodes (and encodes) the course's hidden files.
func rot13(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			c = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			c = 'A' + (c-'A'+13)%26
		}
		out[i] = c
	}
	return out
}

func readROT13(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return rot13(b), nil
}

var hintHeading = regexp.MustCompile(`(?m)^## Hint ([0-9]+)[ \t]*$`)

// splitHints cuts hints.md at the exact headings "## Hint 1" to "## Hint 3",
// like the course's peek tool. Text before "## Hint 1" is the file title.
func splitHints(src []byte) ([3][]byte, error) {
	var out [3][]byte
	locs := hintHeading.FindAllSubmatchIndex(src, -1)
	if len(locs) != 3 {
		return out, fmt.Errorf("hints.md has %d \"## Hint N\" headings, want 3", len(locs))
	}
	for i, loc := range locs {
		if n := string(src[loc[2]:loc[3]]); n != fmt.Sprint(i+1) {
			return out, fmt.Errorf("hints.md: heading %d is \"## Hint %s\"", i+1, n)
		}
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[i] = bytes.TrimSpace(src[loc[1]:end])
		if len(out[i]) == 0 {
			return out, fmt.Errorf("hints.md: hint %d is empty", i+1)
		}
	}
	return out, nil
}

// loadHidden decodes solutions/<slug>/. stubs are the exercise's stub
// file names: the reference solution must have exactly these files, because
// the course's verify script replaces the stubs by name.
func loadHidden(md *markdown, dir, slug string, stubs []string) (*hidden, error) {
	sdir := filepath.Join(dir, "solutions", slug)
	repoDir := "solutions/" + slug + "/"
	h := &hidden{}
	lazyMarkdown := func(src []byte, docPath string) func() (template.HTML, error) {
		return sync.OnceValues(func() (template.HTML, error) { return md.render(src, docPath) })
	}

	src, err := readROT13(filepath.Join(sdir, "hints.md.rot13"))
	if err != nil {
		return nil, err
	}
	hints, err := splitHints(src)
	if err != nil {
		return nil, err
	}
	for i, hint := range hints {
		h.html[Hint1+Level(i)] = lazyMarkdown(hint, repoDir+"hints.md")
	}
	for l, name := range map[Level]string{Explanation: "explanation.md", Walkthrough: "walkthrough.md"} {
		src, err := readROT13(filepath.Join(sdir, name+".rot13"))
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(src)) == 0 {
			return nil, fmt.Errorf("%s is empty", name)
		}
		h.html[l] = lazyMarkdown(src, repoDir+name)
	}

	for _, name := range stubs {
		src, err := readROT13(filepath.Join(sdir, name+".rot13"))
		if err != nil {
			return nil, fmt.Errorf("reference solution: %w", err)
		}
		h.solution = append(h.solution, File{Name: name, Kind: FileStub, Content: string(src)})
	}
	all, err := filepath.Glob(filepath.Join(sdir, "*.go.rot13"))
	if err != nil {
		return nil, err
	}
	if len(all) != len(stubs) {
		return nil, fmt.Errorf("reference solution has %d Go files, the stub has %d", len(all), len(stubs))
	}
	h.html[Solution] = sync.OnceValues(func() (template.HTML, error) {
		var b strings.Builder
		for _, f := range h.solution {
			code, err := highlightGo(f.Content)
			if err != nil {
				return "", fmt.Errorf("highlight %s: %w", f.Name, err)
			}
			fmt.Fprintf(&b, "<h3 class=\"solution-file\">%s</h3>\n%s", template.HTMLEscapeString(f.Name), code)
		}
		return template.HTML(b.String()), nil //nolint:gosec // chroma escapes the code
	})
	return h, nil
}

// highlightGo renders Go source with the same markup that the Markdown
// renderer gives a ```go block.
func highlightGo(src string) (string, error) {
	lexer := lexers.Get("go")
	if lexer == nil {
		return "", errors.New("chroma has no Go lexer")
	}
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	f := chromahtml.New(chromahtml.WithClasses(true))
	if err := f.Format(&buf, styles.Fallback, it); err != nil {
		return "", err
	}
	return buf.String(), nil
}
