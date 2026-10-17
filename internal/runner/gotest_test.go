package runner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestTestStreamEvents(t *testing.T) {
	const pkg = `"Package":"github.com/yornik/go-practice/exercises/01-logfmt"`
	stdout := []string{
		`{"Action":"start",` + pkg + `}`,
		`{"Action":"run",` + pkg + `,"Test":"TestA"}`,
		`{"Action":"output",` + pkg + `,"Test":"TestA","Output":"=== RUN   TestA\n"}`,
		`{"Action":"pause",` + pkg + `,"Test":"TestA"}`,
		`{"Action":"cont",` + pkg + `,"Test":"TestA"}`,
		`{"Action":"output",` + pkg + `,"Test":"TestA","Output":"\t/w/mod/exercises/01-logfmt/logfmt.go:34 +0x1d\n"}`,
		`{"Action":"fail",` + pkg + `,"Test":"TestA","Elapsed":0.25}`,
		`{"Action":"output",` + pkg + `,"Output":"FAIL\n"}`,
		`{"Action":"fail",` + pkg + `,"Elapsed":0.3}`,
		`not json`,
	}
	var got []runnerapi.Event
	s := newTestStream("/w/mod", func(ev runnerapi.Event) { got = append(got, ev) })
	for _, line := range stdout {
		s.stdoutLine([]byte(line + "\n"))
	}
	s.stderrLine([]byte("go: warning: something\n"))

	want := []runnerapi.Event{
		{Type: "test", Action: "run", Test: "TestA"},
		{Type: "test", Action: "output", Test: "TestA", Output: "=== RUN   TestA\n"},
		{Type: "test", Action: "output", Test: "TestA", Output: "\texercises/01-logfmt/logfmt.go:34 +0x1d\n"},
		{Type: "test", Action: "fail", Test: "TestA", Elapsed: 0.25},
		{Type: "test", Action: "output", Output: "FAIL\n"},
		{Type: "test", Action: "fail", Elapsed: 0.3},
		{Type: "test", Action: "output", Output: "not json\n"},
		{Type: "build", Output: "go: warning: something\n"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events:\n got %+v\nwant %+v", got, want)
	}
}

func TestTestStreamBuildFailure(t *testing.T) {
	const ip = `"ImportPath":"github.com/yornik/go-practice/exercises/01-logfmt"`
	lines := []string{
		`{` + ip + `,"Action":"build-output","Output":"# github.com/yornik/go-practice/exercises/01-logfmt\n"}`,
		`{` + ip + `,"Action":"build-output","Output":"exercises/01-logfmt/logfmt.go:105:17: cannot use 1 as string value\n"}`,
		`{` + ip + `,"Action":"build-fail"}`,
		`{"Action":"start","Package":"github.com/yornik/go-practice/exercises/01-logfmt"}`,
		`{"Action":"output","Package":"github.com/yornik/go-practice/exercises/01-logfmt","Output":"FAIL\t... [build failed]\n"}`,
		`{"Action":"fail","Package":"github.com/yornik/go-practice/exercises/01-logfmt","Elapsed":0,"FailedBuild":"github.com/yornik/go-practice/exercises/01-logfmt"}`,
	}
	var builds []string
	s := newTestStream("/w/mod", func(ev runnerapi.Event) {
		if ev.Type == runnerapi.EventBuild {
			builds = append(builds, ev.Output)
		}
	})
	for _, line := range lines {
		s.stdoutLine([]byte(line))
	}
	passed, failed, skipped, buildFailed := s.summary()
	if !buildFailed {
		t.Error("buildFailed = false, want true")
	}
	if passed+failed+skipped != 0 {
		t.Errorf("counts = %d, %d, %d; want all 0", passed, failed, skipped)
	}
	if len(builds) != 2 || !strings.Contains(builds[1], "logfmt.go:105:17") {
		t.Errorf("build output = %q, want the 2 compiler lines", builds)
	}
}

func TestTestStreamSummary(t *testing.T) {
	tests := []struct {
		name                         string
		results                      map[string]string
		wantPass, wantFail, wantSkip int
	}{
		{
			name:     "flat",
			results:  map[string]string{"TestA": "pass", "TestB": "fail", "TestC": "skip"},
			wantPass: 1, wantFail: 1, wantSkip: 1,
		},
		{
			name: "a failing subtest counts once",
			results: map[string]string{
				"TestA": "fail", "TestA/one": "pass", "TestA/two": "fail", "TestA/three": "skip",
			},
			wantPass: 1, wantFail: 1, wantSkip: 1,
		},
		{
			name: "a parent that fails by itself counts",
			results: map[string]string{
				"TestA": "fail", "TestA/one": "pass", "TestA/two": "pass",
			},
			wantPass: 2, wantFail: 1,
		},
		{
			name: "nested subtests count at the leaves",
			results: map[string]string{
				"TestA": "fail", "TestA/x": "fail", "TestA/x/1": "fail", "TestA/x/2": "pass", "TestA/y": "pass",
			},
			wantPass: 2, wantFail: 1,
		},
		{
			name:    "nothing ran",
			results: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStream("/w/mod", func(runnerapi.Event) {})
			s.results = tt.results
			pass, fail, skip, _ := s.summary()
			if pass != tt.wantPass || fail != tt.wantFail || skip != tt.wantSkip {
				t.Errorf("summary = %d, %d, %d; want %d, %d, %d", pass, fail, skip, tt.wantPass, tt.wantFail, tt.wantSkip)
			}
		})
	}
}

func TestAncestors(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"TestA", nil},
		{"TestA/b", []string{"TestA"}},
		{"TestA/b/c", []string{"TestA/b", "TestA"}},
	}
	for _, tt := range tests {
		var got []string
		for a := range ancestors(tt.name) {
			got = append(got, a)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ancestors(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTestStreamAlarm(t *testing.T) {
	const pkg = `"Package":"github.com/yornik/go-practice/exercises/01-logfmt"`
	for _, tt := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{
			name: "go test's alarm",
			lines: []string{
				`{"Action":"run",` + pkg + `,"Test":"TestA"}`,
				`{"Action":"output",` + pkg + `,"Test":"TestA","Output":"panic: test timed out after 35s\n"}`,
				`{"Action":"output",` + pkg + `,"Test":"TestA","Output":"\trunning tests:\n"}`,
				`{"Action":"fail",` + pkg + `,"Elapsed":35.1}`,
			},
			want: true,
		},
		{
			name: "the words inside other output",
			lines: []string{
				`{"Action":"output",` + pkg + `,"Test":"TestA","Output":"    a_test.go:3: got panic: test timed out after 1s\n"}`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStream("/w/mod", func(runnerapi.Event) {})
			for _, line := range tt.lines {
				s.stdoutLine([]byte(line + "\n"))
			}
			if got := s.alarmed(); got != tt.want {
				t.Errorf("alarmed = %v, want %v", got, tt.want)
			}
		})
	}
}
