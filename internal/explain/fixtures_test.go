package explain

import (
	"bufio"
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// fixtureToolchain is the Go release of the runner image (Dockerfile.runner,
// golang:1.27-bookworm). Bump it with the image, so a changed message text
// fails here before it reaches a learner. EXPLAIN_GOTOOLCHAIN overrides it.
const fixtureToolchain = "go1.27.2"

const fixturesDir = "testdata/fixtures"

// fixture is one testdata/fixtures/<dir>: a tiny module and a want file.
type fixture struct {
	dir      string
	id       string   // the catalog entry the output must produce
	command  []string // "build", "vet", "gofmt", "staticcheck" or "test" and go test flags
	file     string   // expected Raw.File, if set
	line     int      // expected Raw.Line, if set
	test     string   // expected Raw.Test, if set
	captures map[string]string
}

// readFixtures parses every want file. The format is documented in
// catalog/README.md.
func readFixtures() ([]fixture, error) {
	dirs, err := os.ReadDir(fixturesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []fixture
	var errs []error
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		fx, err := readWant(d.Name())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, fx)
	}
	return out, errors.Join(errs...)
}

func readWant(dir string) (fixture, error) {
	fx := fixture{dir: dir, captures: make(map[string]string)}
	p := filepath.Join(fixturesDir, dir, "want")
	b, err := os.ReadFile(p)
	if err != nil {
		return fx, err
	}
	for n, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || value == "" {
			return fx, fmt.Errorf("%s:%d: want key: value", p, n+1)
		}
		switch key {
		case "id":
			fx.id = value
		case "command":
			fx.command = strings.Fields(value)
		case "file":
			fx.file = value
		case "line":
			fx.line, err = strconv.Atoi(value)
			if err != nil {
				return fx, fmt.Errorf("%s:%d: %v", p, n+1, err)
			}
		case "test":
			fx.test = value
		case "capture":
			name, v, ok := strings.Cut(value, "=")
			if !ok {
				return fx, fmt.Errorf("%s:%d: want capture: name=value", p, n+1)
			}
			fx.captures[name] = v
		default:
			return fx, fmt.Errorf("%s:%d: unknown key %q", p, n+1, key)
		}
	}
	if fx.id == "" || len(fx.command) == 0 {
		return fx, fmt.Errorf("%s: id and command are required", p)
	}
	if !slices.Contains([]string{"build", "vet", "gofmt", "staticcheck", "test"}, fx.command[0]) {
		return fx, fmt.Errorf("%s: command %q is not build, vet, gofmt, staticcheck or test", p, fx.command[0])
	}
	if fx.command[0] != "test" && len(fx.command) > 1 {
		return fx, fmt.Errorf("%s: only test takes flags", p)
	}
	return fx, nil
}

// TestFixtures runs every fixture with the runner's Go release, feeds the
// real output to the engine and checks that the expected entry explains it
// with the expected values. The fixtures share the build cache and run in
// parallel; -short skips them.
func TestFixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go toolchain; run without -short")
	}
	fixtures, err := readFixtures()
	if err != nil {
		t.Fatal(err)
	}
	toolchain := fixtureToolchain
	if v := os.Getenv("EXPLAIN_GOTOOLCHAIN"); v != "" {
		toolchain = v
	}
	env := fixtureEnv(toolchain)
	cmd := exec.Command("go", "version")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), toolchain+" ") {
		t.Fatalf("toolchain %s is not available (GOTOOLCHAIN=%s go version: %v)\n%s", toolchain, toolchain, err, out)
	}
	staticcheck, why := findStaticcheck(toolchain)
	tools := fixtureTools{staticcheck: staticcheck, parsecheck: buildParsecheck(t, env)}
	for _, fx := range fixtures {
		t.Run(fx.dir, func(t *testing.T) {
			t.Parallel()
			if fx.command[0] == "staticcheck" && staticcheck == "" {
				if os.Getenv("EXPLAIN_REQUIRE_STATICCHECK") != "" {
					t.Fatal(why)
				}
				t.Skip(why)
			}
			exps, output := runFixture(t, fx, env, tools)
			checkFixture(t, fx, exps, output)
		})
	}
}

// fixtureEnv is the environment of the runner (docs/DESIGN.md, "Sandbox
// rules") as far as it changes what the tools print.
func fixtureEnv(toolchain string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GOTOOLCHAIN", "GOFLAGS", "GOWORK", "CGO_ENABLED", "GOTELEMETRY", "GOOS", "GOARCH":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GOTOOLCHAIN="+toolchain, "GOFLAGS=-mod=mod", "GOWORK=off",
		"CGO_ENABLED=1", "GOTELEMETRY=off")
}

// findStaticcheck finds a staticcheck built with the same Go release as
// toolchain: one built with an older release cannot load the newer
// standard library. It returns "" and the reason when there is none.
func findStaticcheck(toolchain string) (string, string) {
	p := os.Getenv("STATICCHECK")
	if p == "" {
		p, _ = exec.LookPath("staticcheck")
	}
	if p == "" {
		if out, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
			candidate := filepath.Join(strings.TrimSpace(string(out)), "bin", "staticcheck")
			if _, err := os.Stat(candidate); err == nil {
				p = candidate
			}
		}
	}
	install := fmt.Sprintf("build one with GOTOOLCHAIN=%s go install honnef.co/go/tools/cmd/staticcheck@v0.8.2 and set STATICCHECK to it", toolchain)
	if p == "" {
		return "", "staticcheck not found; " + install
	}
	info, err := buildinfo.ReadFile(p)
	if err != nil {
		return "", fmt.Sprintf("%s: %v", p, err)
	}
	if minor(info.GoVersion) != minor(toolchain) {
		return "", fmt.Sprintf("%s was built with %s and cannot load %s packages; %s", p, info.GoVersion, toolchain, install)
	}
	return p, ""
}

// minor returns "go1.27" for "go1.27.2".
func minor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// fixtureTools are the programs that fixtures run besides go.
type fixtureTools struct {
	staticcheck string // "" when there is none for this Go release
	parsecheck  string // the runner's syntax check, see testdata/parsecheck
}

// buildParsecheck builds testdata/parsecheck with the fixture toolchain, in
// a module of its own, so its go/parser is the one of the runner's release.
func buildParsecheck(t *testing.T, env []string) string {
	t.Helper()
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "parsecheck", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"go.mod": "module parsecheck\n\ngo 1.24\n", "main.go": string(src)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(dir, "parsecheck")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build parsecheck: %v\n%s", err, out)
	}
	return bin
}

// runFixture copies the fixture to a temporary directory, runs its command
// and explains the output the way the web server does.
func runFixture(t *testing.T, fx fixture, env []string, tools fixtureTools) ([]Explanation, string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(fixturesDir, fx.dir)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == src {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		if rel == "want" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	run := func(name string, args ...string) string {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	switch fx.command[0] {
	case "build":
		out := run("go", "build", "./...")
		_, exps := ExplainBuildOutput(out)
		return exps, out
	case "vet":
		out := run("go", "vet", "-json", "./...")
		var exps []Explanation
		for _, d := range parseVetOutput(t, out) {
			exps = append(exps, Explain(d))
		}
		return exps, out
	case "gofmt":
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for i, f := range files {
			files[i] = filepath.Base(f)
		}
		out := run(tools.parsecheck, files...)
		var exps []Explanation
		for _, d := range parseCompileLines(out, runnerapi.SourceGofmt) {
			exps = append(exps, Explain(d))
		}
		return exps, out
	case "staticcheck":
		out := run(tools.staticcheck, "-f", "json", "./...")
		var exps []Explanation
		for _, d := range parseStaticcheckJSON(t, out) {
			exps = append(exps, Explain(d))
		}
		return exps, out
	default:
		args := append([]string{"test", "-json", "-count=1"}, fx.command[1:]...)
		out := run("go", append(args, "./...")...)
		return explainTestJSON(t, out), out
	}
}

var (
	posnRE        = regexp.MustCompile(`^(.*):(\d+):(\d+)$`)
	compileLineRE = regexp.MustCompile(`^(?:vet: )?([^\s:][^:]*\.go):(\d+)(?::(\d+))?: (.*)$`)
)

// parseVetOutput reads go vet -json output as the runner does
// (internal/runner/diagnostics.go): analyzer findings come as JSON, but
// errors of the type checker as text lines, whose tab-indented
// continuation lines the runner joins with "\n" and no indentation.
func parseVetOutput(t *testing.T, out string) []runnerapi.Diagnostic {
	t.Helper()
	var js, text strings.Builder
	inJSON := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
		case line == "{" || line == "{}":
			inJSON = line == "{"
			js.WriteString(line + "\n")
		case inJSON:
			js.WriteString(line + "\n")
			inJSON = line != "}"
		default:
			text.WriteString(line + "\n")
		}
	}
	diags := parseCompileLines(text.String(), runnerapi.SourceCompile)
	return append(diags, parseVetJSON(t, js.String())...)
}

// parseCompileLines reads "file.go:line:column: message" lines with their
// tab-indented continuation lines, as the runner's check reports them.
func parseCompileLines(out, source string) []runnerapi.Diagnostic {
	var diags []runnerapi.Diagnostic
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "\t") && len(diags) > 0 {
			diags[len(diags)-1].Message += "\n" + strings.TrimSpace(line)
			continue
		}
		m := compileLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		diags = append(diags, runnerapi.Diagnostic{
			File: filepath.Base(m[1]), Line: atoi(m[2]), Column: max(atoi(m[3]), 1),
			Severity: runnerapi.SeverityError, Source: source, Message: m[4],
		})
	}
	return diags
}

// parseVetJSON reads the JSON part of go vet -json output: per package,
// per analyzer, a list of findings. The runner fills Diagnostic.Code with
// the analyzer.
func parseVetJSON(t *testing.T, out string) []runnerapi.Diagnostic {
	t.Helper()
	var diags []runnerapi.Diagnostic
	dec := json.NewDecoder(strings.NewReader(out))
	for {
		var pkgs map[string]map[string]json.RawMessage
		if err := dec.Decode(&pkgs); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("go vet -json: %v\n%s", err, out)
		}
		for _, analyzers := range pkgs {
			for analyzer, raw := range analyzers {
				var findings []struct{ Posn, Message string }
				if err := json.Unmarshal(raw, &findings); err != nil {
					t.Fatalf("go vet -json %s: %v", analyzer, err)
				}
				for _, f := range findings {
					m := posnRE.FindStringSubmatch(f.Posn)
					if m == nil {
						t.Fatalf("go vet -json: position %q", f.Posn)
					}
					diags = append(diags, runnerapi.Diagnostic{
						File: filepath.Base(m[1]), Line: atoi(m[2]), Column: atoi(m[3]),
						Severity: runnerapi.SeverityWarning, Source: runnerapi.SourceVet,
						Code: analyzer, Message: f.Message,
					})
				}
			}
		}
	}
	return diags
}

// parseStaticcheckJSON reads staticcheck -f json output: 1 object a line.
func parseStaticcheckJSON(t *testing.T, out string) []runnerapi.Diagnostic {
	t.Helper()
	var diags []runnerapi.Diagnostic
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "{") {
			continue
		}
		var p struct {
			Code     string
			Message  string
			Location struct {
				File         string
				Line, Column int
			}
		}
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			t.Fatalf("staticcheck -f json: %v\n%s", err, out)
		}
		diags = append(diags, runnerapi.Diagnostic{
			File: filepath.Base(p.Location.File), Line: p.Location.Line, Column: p.Location.Column,
			Severity: runnerapi.SeverityWarning, Source: runnerapi.SourceStaticcheck,
			Code: p.Code, Message: p.Message,
		})
	}
	return diags
}

// explainTestJSON replays go test -json output the way the web server sees
// the runner's stream: build output first, then the output of every test
// that did not pass, including package-level output (test "").
func explainTestJSON(t *testing.T, out string) []Explanation {
	t.Helper()
	var build strings.Builder
	outputs := make(map[string]*strings.Builder)
	var order []string
	final := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var ev struct{ Action, Test, Output string }
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("go test -json: %v: %s", err, line)
		}
		switch ev.Action {
		case "build-output":
			build.WriteString(ev.Output)
		case "output":
			b, ok := outputs[ev.Test]
			if !ok {
				b = new(strings.Builder)
				outputs[ev.Test] = b
				order = append(order, ev.Test)
			}
			b.WriteString(ev.Output)
		case "pass", "fail", "skip":
			final[ev.Test] = ev.Action
		}
	}
	var exps []Explanation
	if build.Len() > 0 {
		_, e := ExplainBuildOutput(build.String())
		exps = append(exps, e...)
	}
	for _, test := range order {
		if a := final[test]; a == "pass" || a == "skip" {
			continue
		}
		exps = append(exps, ExplainTestOutput(test, outputs[test].String())...)
	}
	return exps
}

func checkFixture(t *testing.T, fx fixture, exps []Explanation, output string) {
	t.Helper()
	var problems []string
	for _, e := range exps {
		if e.ID != fx.id {
			continue
		}
		var p []string
		if fx.file != "" && e.Raw.File != fx.file {
			p = append(p, fmt.Sprintf("file %q, want %q", e.Raw.File, fx.file))
		}
		if fx.line != 0 && e.Raw.Line != fx.line {
			p = append(p, fmt.Sprintf("line %d, want %d", e.Raw.Line, fx.line))
		}
		if fx.test != "" && e.Raw.Test != fx.test {
			p = append(p, fmt.Sprintf("test %q, want %q", e.Raw.Test, fx.test))
		}
		for k, v := range fx.captures {
			if got := e.Raw.Captures[k]; got != v {
				p = append(p, fmt.Sprintf("{{%s}} = %q, want %q", k, got, v))
			}
		}
		if strings.Contains(e.HTML, "{{") || strings.Contains(e.Title, "{{") {
			p = append(p, "a placeholder is not filled in")
		}
		if len(p) == 0 {
			return
		}
		problems = append(problems, strings.Join(p, "; "))
	}
	var got []string
	for _, e := range exps {
		got = append(got, fmt.Sprintf("%s (%q)", e.ID, e.Raw.Message))
	}
	if len(output) > 4000 {
		output = output[:4000] + "\n[...]"
	}
	if len(problems) > 0 {
		t.Fatalf("%s matched with the wrong values: %s\noutput:\n%s", fx.id, strings.Join(problems, " | "), output)
	}
	t.Fatalf("no explanation %s; got %v\noutput:\n%s", fx.id, got, output)
}
