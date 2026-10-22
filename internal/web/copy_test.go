package web_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// codeLike finds Go code in prose: a call such as Validate() or an index
// such as x[i]. Code belongs in <code>, which is set in Atkinson
// Hyperlegible Mono, so a reader can tell it from the prose around it.
var codeLike = regexp.MustCompile(`\b[A-Za-z_][\w.]*\([^()\s]*\)|\b[A-Za-z_]\w*\[[\w:.]*\]`)

var (
	tmplAction = regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	codeOrPre  = regexp.MustCompile(`(?s)<(code|pre|script|style)\b.*?</(code|pre|script|style)>`)
	anyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
)

// TestTemplateCodeInCode keeps code in the templates' prose inside <code>.
func TestTemplateCodeInCode(t *testing.T) {
	root := filepath.Join("..", "..", "web", "templates")
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return err
		}
		text := tmplAction.ReplaceAllString(string(b), " ")
		text = codeOrPre.ReplaceAllString(text, " ")
		text = anyTag.ReplaceAllString(text, " ")
		for _, line := range strings.Split(text, "\n") {
			if m := codeLike.FindString(line); m != "" {
				t.Errorf("%s: %q is code outside <code> in: %s", path, m, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
