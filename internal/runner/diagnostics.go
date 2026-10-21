package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// compileLineRE matches a compiler or type checker error as go vet prints
// it, for example "exercises/32-retry/retry.go:12:5: undefined: foo" or,
// for the external test package, "vet: exercises/32-retry/retry_test.go:3:2:
// ...". The column is optional because some errors have only a line.
var compileLineRE = regexp.MustCompile(`^(?:vet: )?([^\s:][^:]*\.go):(\d+)(?::(\d+))?: (.*)$`)

// posnRE splits a "file:line:col" or "file:line" position from go vet -json.
var posnRE = regexp.MustCompile(`^(.*\.go):(\d+)(?::(\d+))?$`)

// vetFinding is one diagnostic in go vet -json output.
type vetFinding struct {
	Posn    string `json:"posn"`
	End     string `json:"end"`
	Message string `json:"message"`
}

// parseVetOutput turns the output of go vet -json into diagnostics.
//
// go vet -json prints analyzer findings as indented JSON objects, keyed by
// package and analyzer, but compile and type errors as plain text lines, in
// the same stream and between "# package" headers. Text errors become
// compile errors; JSON findings become vet warnings with the analyzer name
// as the code. File names are reduced to their base name: every file of
// interest lives in the exercise directory.
func parseVetOutput(out []byte) []runnerapi.Diagnostic {
	var diags []runnerapi.Diagnostic
	var block []byte // the JSON object being collected
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, len(out)+1)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case block != nil:
			block = append(block, line...)
			block = append(block, '\n')
			if line == "}" {
				diags = append(diags, parseVetJSON(block)...)
				block = nil
			}
		case line == "{":
			block = []byte("{\n")
		case line == "{}", strings.HasPrefix(line, "#"):
			// An empty result or a package header.
		case strings.HasPrefix(line, "\t") && len(diags) > 0:
			// Continuation of the previous error, for example the
			// "have (int)" and "want (string)" lines of a call mismatch.
			d := &diags[len(diags)-1]
			d.Message += "\n" + strings.TrimSpace(line)
		default:
			if d, ok := parseCompileLine(line); ok {
				diags = append(diags, d)
			}
		}
	}
	return diags
}

func parseCompileLine(line string) (runnerapi.Diagnostic, bool) {
	m := compileLineRE.FindStringSubmatch(line)
	if m == nil {
		return runnerapi.Diagnostic{}, false
	}
	return runnerapi.Diagnostic{
		File:     filepath.Base(m[1]),
		Line:     atoi(m[2]),
		Column:   max(atoi(m[3]), 1),
		Severity: runnerapi.SeverityError,
		Source:   runnerapi.SourceCompile,
		Message:  m[4],
	}, true
}

// parseVetJSON decodes 1 JSON object of go vet -json output. An analyzer
// that failed reports {"error": "..."} instead of a list; such entries
// carry no position and are left out.
func parseVetJSON(block []byte) []runnerapi.Diagnostic {
	var byPkg map[string]map[string]json.RawMessage
	if err := json.Unmarshal(block, &byPkg); err != nil {
		return nil
	}
	var diags []runnerapi.Diagnostic
	for _, byAnalyzer := range byPkg {
		for analyzer, raw := range byAnalyzer {
			var findings []vetFinding
			if err := json.Unmarshal(raw, &findings); err != nil {
				continue
			}
			for _, f := range findings {
				file, line, col, ok := splitPosn(f.Posn)
				if !ok {
					continue
				}
				d := runnerapi.Diagnostic{
					File:     file,
					Line:     line,
					Column:   max(col, 1),
					Severity: runnerapi.SeverityWarning,
					Source:   runnerapi.SourceVet,
					Code:     analyzer,
					Message:  f.Message,
				}
				if endFile, endLine, endCol, ok := splitPosn(f.End); ok && endFile == file {
					d.EndLine, d.EndColumn = endLine, endCol
				}
				diags = append(diags, d)
			}
		}
	}
	return diags
}

func splitPosn(posn string) (file string, line, col int, ok bool) {
	m := posnRE.FindStringSubmatch(posn)
	if m == nil {
		return "", 0, 0, false
	}
	return filepath.Base(m[1]), atoi(m[2]), atoi(m[3]), true
}

// staticcheckPosition is a position in staticcheck -f json output.
type staticcheckPosition struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// staticcheckFinding is one line of staticcheck -f json output.
type staticcheckFinding struct {
	Code     string              `json:"code"`
	Severity string              `json:"severity"`
	Location staticcheckPosition `json:"location"`
	End      staticcheckPosition `json:"end"`
	Message  string              `json:"message"`
	// Related are further positions that explain the finding, for example
	// SA4023's "the lhs of the comparison gets its value from here".
	Related []struct {
		Location staticcheckPosition `json:"location"`
		End      staticcheckPosition `json:"end"`
		Message  string              `json:"message"`
	} `json:"related"`
}

// parseStaticcheckOutput turns staticcheck -f json output (1 object per
// line) into diagnostics. keep filters by base file name.
//
// staticcheck labels every finding "error" by default; the runner reports
// them as warnings, as for vet, and keeps "error" for code that does not
// compile (code "compile").
func parseStaticcheckOutput(out []byte, keep func(file string) bool) []runnerapi.Diagnostic {
	var diags []runnerapi.Diagnostic
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, len(out)+1)
	for sc.Scan() {
		var f staticcheckFinding
		if err := json.Unmarshal(sc.Bytes(), &f); err != nil || f.Location.File == "" {
			continue
		}
		file := filepath.Base(f.Location.File)
		if !keep(file) {
			continue
		}
		d := runnerapi.Diagnostic{
			File:     file,
			Line:     max(f.Location.Line, 1),
			Column:   max(f.Location.Column, 1),
			Severity: runnerapi.SeverityWarning,
			Source:   runnerapi.SourceStaticcheck,
			Code:     f.Code,
			Message:  f.Message,
		}
		if f.Code == "compile" {
			d.Severity, d.Source, d.Code = runnerapi.SeverityError, runnerapi.SourceCompile, ""
		}
		if f.End.File != "" && filepath.Base(f.End.File) == file {
			d.EndLine, d.EndColumn = f.End.Line, f.End.Column
		}
		d.Related = staticcheckRelated(f)
		diags = append(diags, d)
	}
	return diags
}

// staticcheckRelated returns the first related position of a finding when
// it lies in the exercise directory, the directory of the finding itself:
// a position in the standard library or the module cache is no place the
// learner can open. The UI shows 1; the first is the one the message
// refers to.
func staticcheckRelated(f staticcheckFinding) *runnerapi.Related {
	if len(f.Related) == 0 {
		return nil
	}
	rel := f.Related[0]
	if rel.Location.File == "" || filepath.Dir(rel.Location.File) != filepath.Dir(f.Location.File) {
		return nil
	}
	file := filepath.Base(rel.Location.File)
	r := &runnerapi.Related{
		File:    file,
		Line:    max(rel.Location.Line, 1),
		Column:  max(rel.Location.Column, 1),
		Message: rel.Message,
	}
	if rel.End.File != "" && filepath.Base(rel.End.File) == file {
		r.EndLine, r.EndColumn = rel.End.Line, rel.End.Column
	}
	return r
}

// atoi converts the digits a regular expression matched; an empty optional
// group gives 0.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
