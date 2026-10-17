package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

const testToken = "test-token-0123456789abcdef0123456789abcdef"

// requestTimeout bounds every request a test makes. A cold build cache
// compiles the standard library with -race, which takes a while once.
const requestTimeout = 3 * time.Minute

// toolchain is the go command and caches the tests use. go test puts its
// own GOROOT/bin first in PATH, so the runner under test uses the same go.
// The tests share the developer's build cache: a private one would compile
// the standard library with -race in every run.
var toolchain = sync.OnceValues(func() (toolchainInfo, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return toolchainInfo{}, err
	}
	out, err := exec.Command(goPath, "env", "GOCACHE", "GOPATH").Output()
	if err != nil {
		return toolchainInfo{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	info := toolchainInfo{goPath: goPath, goCache: lines[0]}
	if sc, err := exec.LookPath("staticcheck"); err == nil {
		info.staticcheck = sc
	} else if len(lines) > 1 {
		if sc := filepath.Join(lines[1], "bin", "staticcheck"); isExecutable(sc) {
			info.staticcheck = sc
		}
	}
	return info, nil
})

type toolchainInfo struct {
	goPath, goCache, staticcheck string
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// slow skips a test that runs the go toolchain when -short is set.
func slow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go toolchain; skipped with -short")
	}
}

// testEnv is a Server behind an httptest.Server.
type testEnv struct {
	srv     *Server
	url     string
	workDir string
	course  *Course
}

// newTestEnv starts a server on the real course. Options can be adjusted
// before the server is built.
func newTestEnv(t *testing.T, adjust ...func(*Options)) *testEnv {
	t.Helper()
	tc, err := toolchain()
	if err != nil {
		t.Fatalf("find the go toolchain: %v", err)
	}
	course := loadTestCourse(t)
	opts := Options{
		Token:            testToken,
		Course:           course,
		WorkDir:          t.TempDir(),
		GoCache:          tc.goCache,
		Go:               tc.goPath,
		Staticcheck:      tc.staticcheck,
		StaticcheckCache: filepath.Join(filepath.Dir(tc.goCache), "staticcheck"),
		MaxConcurrent:    2,
		MaxQueue:         4,
		TestTimeout:      2 * time.Minute,
		CheckTimeout:     2 * time.Minute,
	}
	for _, f := range adjust {
		f(&opts)
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// As cmd/runner does before it listens.
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if _, err := srv.Probe(ctx); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		hs.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Close(ctx); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return &testEnv{srv: srv, url: hs.URL, workDir: opts.WorkDir, course: course}
}

// get sends a GET request with the given Authorization header (none when
// empty) and returns the status and the body.
func (e *testEnv) get(t *testing.T, path, authorization string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s answer: %v", path, err)
	}
	return resp.StatusCode, body
}

// post sends body (a value to encode, or a string sent as is) with the
// test token.
func (e *testEnv) post(t *testing.T, ctx context.Context, path string, body any) *http.Response {
	t.Helper()
	var data []byte
	switch b := body.(type) {
	case string:
		data = []byte(b)
	default:
		var err error
		if data, err = json.Marshal(b); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// postJSON posts body and decodes a JSON answer into out, checking the
// status.
func (e *testEnv) postJSON(t *testing.T, path string, body, out any, wantStatus int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	resp := e.post(t, ctx, path, body)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s answer: %v", path, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s: status %d, want %d; body %s", path, resp.StatusCode, wantStatus, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("decode %s answer %q: %v", path, data, err)
		}
	}
}

// runTests posts a test request and collects the event stream.
func (e *testEnv) runTests(t *testing.T, req runnerapi.TestRequest) []runnerapi.Event {
	t.Helper()
	events, err := e.collectTests(req)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// collectTests is runTests for a goroutine other than the test's own,
// which must not call t.Fatal.
func (e *testEnv) collectTests(req runnerapi.TestRequest) ([]runnerapi.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+runnerapi.PathTest, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("test request: status %d: %s", resp.StatusCode, data)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		return nil, fmt.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}
	events, err := readEvents(resp.Body, nil)
	if err != nil {
		return events, fmt.Errorf("read events: %w", err)
	}
	return events, nil
}

// readEvents decodes NDJSON events until EOF, calling each (if not nil) as
// they arrive.
func readEvents(r io.Reader, each func(runnerapi.Event)) ([]runnerapi.Event, error) {
	var events []runnerapi.Event
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 4<<20)
	for sc.Scan() {
		var ev runnerapi.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return events, err
		}
		events = append(events, ev)
		if each != nil {
			each(ev)
		}
	}
	return events, sc.Err()
}

// doneEvent returns the last event, which must be the done event.
func doneEvent(t *testing.T, events []runnerapi.Event) runnerapi.Event {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	done := events[len(events)-1]
	if done.Type != runnerapi.EventDone {
		t.Fatalf("last event is %+v, want a done event", done)
	}
	return done
}

// stubFiles returns the exercise's stub files from the course.
func stubFiles(t *testing.T, ex string) map[string]string {
	t.Helper()
	dir := filepath.Join(courseDir, "exercises", ex)
	c := loadTestCourse(t)
	e, ok := c.Exercise(ex)
	if !ok {
		t.Fatalf("no exercise %s", ex)
	}
	files := make(map[string]string)
	for _, name := range e.Stubs {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	return files
}

// solutionFiles decodes the hidden reference solution of an exercise.
func solutionFiles(t *testing.T, ex string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(courseDir, "solutions", ex, "*.go.rot13"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no solution for %s: %v", ex, err)
	}
	files := make(map[string]string)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[strings.TrimSuffix(filepath.Base(p), ".rot13")] = rot13(string(data))
	}
	return files
}

func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

// processesUnder returns the PIDs of live processes whose working
// directory is under dir, with their command line. The runner starts every
// child in a workspace under its work dir, so this finds go, the test
// binary and anything they started. Zombies have no working directory and
// do not count.
func processesUnder(dir string) map[int]string {
	procs := make(map[int]string)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return procs
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || !strings.HasPrefix(cwd, dir) {
			continue
		}
		cmdline, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		procs[pid] = strings.ReplaceAll(string(cmdline), "\x00", " ")
	}
	return procs
}

// waitFor polls cond every 50 ms until it is true or timeout passes.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// testBinaryRunning reports whether a test binary runs under dir.
func testBinaryRunning(dir string) bool {
	for _, cmd := range processesUnder(dir) {
		first, _, _ := strings.Cut(cmd, " ")
		if strings.HasSuffix(first, ".test") {
			return true
		}
	}
	return false
}

// readDirNames lists the names in dir.
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, err
}
