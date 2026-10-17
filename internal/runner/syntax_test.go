package runner

import (
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestSyntaxDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLine  int
		wantCol   int
		wantInMsg string
	}{
		{
			name:      "missing closing paren",
			src:       "package p\n\nfunc f() {\n\tx := (1 + 2\n}\n",
			wantLine:  4,
			wantCol:   13,
			wantInMsg: "expected ')'",
		},
		{
			name:      "no package clause",
			src:       "func f() {}\n",
			wantLine:  1,
			wantCol:   1,
			wantInMsg: "expected 'package'",
		},
		{
			name:      "column counts bytes after a tab",
			src:       "package p\n\nvar x = \t]\n",
			wantLine:  3,
			wantCol:   10,
			wantInMsg: "expected operand",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := syntaxDiagnostics("f.go", tt.src)
			if len(diags) == 0 {
				t.Fatal("no diagnostics for a file that does not parse")
			}
			d := diags[0]
			if d.File != "f.go" || d.Line != tt.wantLine || d.Column != tt.wantCol {
				t.Errorf("position = %s:%d:%d, want f.go:%d:%d", d.File, d.Line, d.Column, tt.wantLine, tt.wantCol)
			}
			if d.Source != runnerapi.SourceGofmt || d.Severity != runnerapi.SeverityError {
				t.Errorf("source, severity = %s, %s; want gofmt, error", d.Source, d.Severity)
			}
			if !strings.Contains(d.Message, tt.wantInMsg) {
				t.Errorf("message = %q, want it to contain %q", d.Message, tt.wantInMsg)
			}
		})
	}
	if diags := syntaxDiagnostics("ok.go", "package p\n"); diags != nil {
		t.Errorf("diagnostics for a valid file: %+v", diags)
	}
}

func TestFormatFile(t *testing.T) {
	got, diags, err := formatFile("f.go", "package p\nimport (\"os\"\n\"fmt\")\nfunc f( ) { fmt.Println( os.Args ) }\n")
	if err != nil || diags != nil {
		t.Fatalf("formatFile: diags %v, err %v", diags, err)
	}
	want := "package p\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc f() { fmt.Println(os.Args) }\n"
	if got != want {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, want)
	}

	bad := "package p\nfunc f( {\n"
	got, diags, err = formatFile("f.go", bad)
	if err != nil {
		t.Fatalf("formatFile on a syntax error: %v", err)
	}
	if got != bad {
		t.Errorf("content changed on a syntax error: %q", got)
	}
	if len(diags) == 0 || diags[0].Line != 2 {
		t.Errorf("diagnostics = %+v, want an error on line 2", diags)
	}
}
