package course

import (
	"go/ast"
	"go/parser"
	"go/token"
	"html"
	"html/template"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// identWord is a Go identifier, optionally qualified: Validate, Policy.Validate.
var identWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?`)

// StepLabel is the label of a step as HTML: the first letter in upper
// case, and the names the stubs declare (Validate, ParseRetryAfter) in
// <code>, because the UI sets code in mono and prose in B612.
func (e *Exercise) StepLabel(s Step) template.HTML {
	label := s.Label
	if label == "" {
		return template.HTML(html.EscapeString(s.Title()))
	}
	names := e.declaredNames()
	var b strings.Builder
	last := 0
	for i, m := range identWord.FindAllStringIndex(label, -1) {
		word := label[m[0]:m[1]]
		code := names[word]
		if !code {
			if a, c, ok := strings.Cut(word, "."); ok {
				code = names[a] && names[c]
			}
		}
		b.WriteString(html.EscapeString(label[last:m[0]]))
		switch {
		case code:
			b.WriteString("<code>" + html.EscapeString(word) + "</code>")
		case i == 0 && m[0] == 0:
			r, size := utf8.DecodeRuneInString(word)
			b.WriteString(html.EscapeString(string(unicode.ToUpper(r)) + word[size:]))
		default:
			b.WriteString(html.EscapeString(word))
		}
		last = m[1]
	}
	b.WriteString(html.EscapeString(label[last:]))
	return template.HTML(b.String()) // every piece is escaped above
}

// declaredNames returns the exported names the stub files declare at the
// top level, the exported methods, and the tests.
func (e *Exercise) declaredNames() map[string]bool {
	e.namesOnce.Do(func() {
		e.names = map[string]bool{}
		for _, t := range e.Tests {
			e.names[t] = true
		}
		fset := token.NewFileSet()
		for _, f := range e.Files {
			if f.Kind != FileStub {
				continue
			}
			af, err := parser.ParseFile(fset, f.Name, f.Content, parser.SkipObjectResolution)
			if err != nil {
				continue // a stub that does not parse has no names to mark
			}
			for _, d := range af.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					e.addName(d.Name.Name)
				case *ast.GenDecl:
					for _, sp := range d.Specs {
						switch sp := sp.(type) {
						case *ast.TypeSpec:
							e.addName(sp.Name.Name)
						case *ast.ValueSpec:
							for _, n := range sp.Names {
								e.addName(n.Name)
							}
						}
					}
				}
			}
		}
	})
	return e.names
}

func (e *Exercise) addName(n string) {
	if token.IsExported(n) {
		e.names[n] = true
	}
}
