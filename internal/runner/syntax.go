package runner

import (
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"maps"
	"slices"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// syntaxDiagnostics parses one Go file and returns its syntax errors as
// gofmt diagnostics. It returns nil for a file that parses.
//
// The parser runs without parser.AllErrors on purpose: it then reports at
// most 1 error per line and stops after 10, which is what a learner can act
// on, rather than the cascade that follows a missing brace.
func syntaxDiagnostics(name, src string) []runnerapi.Diagnostic {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err == nil {
		return nil
	}
	var list scanner.ErrorList
	if !errors.As(err, &list) {
		return []runnerapi.Diagnostic{{
			File: name, Line: 1, Column: 1,
			Severity: runnerapi.SeverityError, Source: runnerapi.SourceGofmt,
			Message: err.Error(),
		}}
	}
	diags := make([]runnerapi.Diagnostic, 0, len(list))
	for _, e := range list {
		diags = append(diags, runnerapi.Diagnostic{
			File:     name,
			Line:     max(e.Pos.Line, 1),
			Column:   max(e.Pos.Column, 1),
			Severity: runnerapi.SeverityError,
			Source:   runnerapi.SourceGofmt,
			Message:  e.Msg,
		})
	}
	return diags
}

// syntaxCheck runs syntaxDiagnostics on every file, in name order.
func syntaxCheck(files map[string]string) []runnerapi.Diagnostic {
	var diags []runnerapi.Diagnostic
	for _, name := range slices.Sorted(maps.Keys(files)) {
		diags = append(diags, syntaxDiagnostics(name, files[name])...)
	}
	return diags
}

// formatFile formats src like gofmt. On a syntax error it returns src
// unchanged with the diagnostics. The syntax check comes first because
// format.Source retries a file without a package clause as a fragment and
// would report positions shifted by its own wrapper.
func formatFile(name, src string) (string, []runnerapi.Diagnostic, error) {
	if diags := syntaxDiagnostics(name, src); len(diags) > 0 {
		return src, diags, nil
	}
	out, err := format.Source([]byte(src))
	if err != nil {
		// The file parsed a moment ago, so this is a printer failure.
		return src, nil, fmt.Errorf("format %s: %w", name, err)
	}
	return string(out), nil, nil
}
