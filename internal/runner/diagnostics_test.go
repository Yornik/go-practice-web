package runner

import (
	"reflect"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestParseVetOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []runnerapi.Diagnostic
	}{
		{
			name: "clean",
			out:  "",
			want: nil,
		},
		{
			name: "compile errors with a package header (go 1.24)",
			out: "# github.com/yornik/go-practice/exercises/01-logfmt\n" +
				"exercises/01-logfmt/logfmt.go:104:6: declared and not used: x\n" +
				"exercises/01-logfmt/logfmt.go:106:2: undefined: undefinedFn\n",
			want: []runnerapi.Diagnostic{
				{File: "logfmt.go", Line: 104, Column: 6, Severity: "error", Source: "compile", Message: "declared and not used: x"},
				{File: "logfmt.go", Line: 106, Column: 2, Severity: "error", Source: "compile", Message: "undefined: undefinedFn"},
			},
		},
		{
			name: "type error in the external test package",
			out: "# github.com/yornik/go-practice/exercises/01-logfmt_test\n" +
				"# [github.com/yornik/go-practice/exercises/01-logfmt_test]\n" +
				"vet: exercises/01-logfmt/logfmt_test.go:38:23: undefined: logfmt.Parse\n",
			want: []runnerapi.Diagnostic{
				{File: "logfmt_test.go", Line: 38, Column: 23, Severity: "error", Source: "compile", Message: "undefined: logfmt.Parse"},
			},
		},
		{
			name: "continuation lines join the message",
			out: "exercises/07-retry/retry.go:12:9: not enough arguments in call to f\n" +
				"\thave ()\n" +
				"\twant (int)\n",
			want: []runnerapi.Diagnostic{
				{File: "retry.go", Line: 12, Column: 9, Severity: "error", Source: "compile",
					Message: "not enough arguments in call to f\nhave ()\nwant (int)"},
			},
		},
		{
			name: "analyzer findings as JSON (go 1.26, with end)",
			out: "{\n" +
				"\t\"github.com/yornik/go-practice/exercises/01-logfmt\": {\n" +
				"\t\t\"printf\": [\n" +
				"\t\t\t{\n" +
				"\t\t\t\t\"posn\": \"/tmp/runs/run-1/mod/exercises/01-logfmt/logfmt.go:105:14\",\n" +
				"\t\t\t\t\"end\": \"/tmp/runs/run-1/mod/exercises/01-logfmt/logfmt.go:105:16\",\n" +
				"\t\t\t\t\"message\": \"fmt.Printf format %d has arg s of wrong type string\"\n" +
				"\t\t\t}\n" +
				"\t\t]\n" +
				"\t}\n" +
				"}\n" +
				"{}\n",
			want: []runnerapi.Diagnostic{
				{File: "logfmt.go", Line: 105, Column: 14, EndLine: 105, EndColumn: 16, Severity: "warning",
					Source: "vet", Code: "printf", Message: "fmt.Printf format %d has arg s of wrong type string"},
			},
		},
		{
			name: "JSON findings next to a type error (go 1.24 headers)",
			out: "# github.com/yornik/go-practice/exercises/01-logfmt\n" +
				"{\n" +
				"\t\"github.com/yornik/go-practice/exercises/01-logfmt\": {\n" +
				"\t\t\"printf\": [\n" +
				"\t\t\t{\n" +
				"\t\t\t\t\"posn\": \"/w/mod/exercises/01-logfmt/logfmt.go:105:2\",\n" +
				"\t\t\t\t\"message\": \"bad format\"\n" +
				"\t\t\t}\n" +
				"\t\t],\n" +
				"\t\t\"tests\": {\"error\": \"analysis skipped\"}\n" +
				"\t}\n" +
				"}\n" +
				"# github.com/yornik/go-practice/exercises/01-logfmt_test\n" +
				"vet: exercises/01-logfmt/logfmt_test.go:38:23: undefined: logfmt.Parse\n",
			want: []runnerapi.Diagnostic{
				{File: "logfmt.go", Line: 105, Column: 2, Severity: "warning", Source: "vet", Code: "printf", Message: "bad format"},
				{File: "logfmt_test.go", Line: 38, Column: 23, Severity: "error", Source: "compile", Message: "undefined: logfmt.Parse"},
			},
		},
		{
			name: "noise is ignored",
			out:  "go: downloading nothing\nsomething else\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVetOutput([]byte(tt.out))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseVetOutput:\n got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseStaticcheckOutput(t *testing.T) {
	out := `{"code":"U1000","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":103,"column":6},"end":{"file":"","line":0,"column":0},"message":"func bad is unused"}
{"code":"SA5009","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":104,"column":13},"end":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":104,"column":19},"message":"Printf format %d has arg #1 of wrong type string"}
{"code":"SA4006","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt_test.go","line":10,"column":2},"end":{"file":"","line":0,"column":0},"message":"this value of x is never used"}
{"code":"compile","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":3,"column":1},"end":{"file":"","line":0,"column":0},"message":"undefined: x"}
{"code":"SA4023","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":16,"column":5},"end":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":16,"column":15},"message":"this comparison is never true","related":[{"location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":15,"column":18},"end":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":15,"column":32},"message":"the lhs of the comparison gets its value from here and has a concrete type"},{"location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":9,"column":1},"end":{"file":"","line":0,"column":0},"message":"second"}]}
{"code":"SA1012","severity":"error","location":{"file":"/w/mod/exercises/01-logfmt/logfmt.go","line":20,"column":2},"end":{"file":"","line":0,"column":0},"message":"do not pass a nil Context","related":[{"location":{"file":"/usr/local/go/src/context/context.go","line":70,"column":1},"end":{"file":"","line":0,"column":0},"message":"in the standard library"}]}
not json
`
	keep := func(file string) bool { return file == "logfmt.go" }
	got := parseStaticcheckOutput([]byte(out), keep)
	want := []runnerapi.Diagnostic{
		{File: "logfmt.go", Line: 103, Column: 6, Severity: "warning", Source: "staticcheck", Code: "U1000", Message: "func bad is unused"},
		{File: "logfmt.go", Line: 104, Column: 13, EndLine: 104, EndColumn: 19, Severity: "warning", Source: "staticcheck", Code: "SA5009",
			Message: "Printf format %d has arg #1 of wrong type string"},
		{File: "logfmt.go", Line: 3, Column: 1, Severity: "error", Source: "compile", Message: "undefined: x"},
		// The first related position stays when it is in the exercise.
		{File: "logfmt.go", Line: 16, Column: 5, EndLine: 16, EndColumn: 15, Severity: "warning", Source: "staticcheck", Code: "SA4023",
			Message: "this comparison is never true",
			Related: &runnerapi.Related{File: "logfmt.go", Line: 15, Column: 18, EndLine: 15, EndColumn: 32,
				Message: "the lhs of the comparison gets its value from here and has a concrete type"}},
		// 1 in the standard library is no place to open.
		{File: "logfmt.go", Line: 20, Column: 2, Severity: "warning", Source: "staticcheck", Code: "SA1012",
			Message: "do not pass a nil Context"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseStaticcheckOutput:\n got %+v\nwant %+v", got, want)
		for i := range min(len(got), len(want)) {
			if !reflect.DeepEqual(got[i].Related, want[i].Related) {
				t.Errorf("diagnostic %d: related %+v, want %+v", i, got[i].Related, want[i].Related)
			}
		}
	}
}

func TestSortDiagnostics(t *testing.T) {
	diags := []runnerapi.Diagnostic{
		{File: "b.go", Line: 1, Column: 1, Message: "b"},
		{File: "a.go", Line: 9, Column: 1, Message: "a9"},
		{File: "a.go", Line: 2, Column: 5, Message: "a2.5 first"},
		{File: "a.go", Line: 2, Column: 5, Message: "a2.5 second"},
		{File: "a.go", Line: 2, Column: 1, Message: "a2.1"},
	}
	sortDiagnostics(diags)
	var got []string
	for _, d := range diags {
		got = append(got, d.Message)
	}
	want := []string{"a2.1", "a2.5 first", "a2.5 second", "a9", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
