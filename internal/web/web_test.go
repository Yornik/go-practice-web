package web_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/runnerapi"
)

func TestCSRFAndOrigin(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	tests := []struct {
		name   string
		header map[string]string
		code   string
	}{
		{"no CSRF header", map[string]string{"X-GPW-CSRF": ""}, "csrf"},
		{"wrong CSRF value", map[string]string{"X-GPW-CSRF": "yes"}, "csrf"},
		{"no Origin", map[string]string{"Origin": ""}, "origin"},
		{"other origin", map[string]string{"Origin": "https://evil.example"}, "origin"},
		{"origin with a path trick", map[string]string{"Origin": "https://go-learn.party.evil.example"}, "origin"},
		{"http origin", map[string]string{"Origin": "http://go-learn.party"}, "origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, r := range []req{
				{method: "POST", path: "/api/auth/login/begin", body: map[string]any{}},
				{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Eve"}, as: admin},
				{method: "DELETE", path: "/api/admin/invites/00", as: admin},
				{method: "POST", path: "/api/auth/logout", body: map[string]any{}, as: admin},
			} {
				r.header = tt.header
				w := h.do(r)
				wantStatus(t, w, http.StatusForbidden)
				if got := decodeJSON[map[string]string](t, w)["code"]; got != tt.code {
					t.Fatalf("%s %s: code %q, want %q", r.method, r.path, got, tt.code)
				}
			}
		})
	}
	// The same requests pass with the header and the Origin.
	wantStatus(t, h.do(req{method: "POST", path: "/api/auth/login/begin", body: map[string]any{}}), http.StatusOK)
	u, _ := h.store.User(t.Context(), admin.ID)
	if u.DisplayName != "Ada Admin" {
		t.Fatalf("a refused request changed the name to %q", u.DisplayName)
	}
}

func TestAuthRequired(t *testing.T) {
	h := newHarness(t)
	for _, r := range []req{
		{method: "GET", path: "/api/exercises/" + logfmt},
		{method: "PUT", path: "/api/exercises/" + logfmt + "/files/logfmt.go", body: map[string]string{"content": "x"}},
		{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{}},
		{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}},
		{method: "POST", path: "/api/exercises/" + logfmt + "/reveal", body: map[string]int{"level": 1}},
		{method: "POST", path: "/api/exercises/" + logfmt + "/heartbeat", body: map[string]any{}},
		{method: "GET", path: "/api/certificates"},
		{method: "POST", path: "/api/certificates", body: map[string]string{"kind": "tier1"}},
		{method: "PUT", path: "/api/account", body: map[string]string{}},
		{method: "POST", path: "/api/auth/passkeys/begin", body: map[string]any{}},
		{method: "POST", path: "/api/admin/invites", body: map[string]any{}},
		{method: "GET", path: "/api/docs/pkg?path=strings"},
	} {
		w := h.do(r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", r.method, r.path, w.Code)
		}
	}
	for _, p := range []string{"/learn", "/learn/" + logfmt, "/account", "/certificates", "/admin"} {
		w := h.do(req{method: "GET", path: p})
		if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/login?next=") {
			t.Errorf("GET %s = %d %s, want a redirect to /login", p, w.Code, w.Header().Get("Location"))
		}
	}

	admin := h.admin()
	bob := h.learner(admin, "bob")
	for _, r := range []req{
		{method: "POST", path: "/api/admin/invites", body: map[string]any{}, as: bob},
		{method: "DELETE", path: "/api/admin/invites/abcd", as: bob},
		{method: "PUT", path: "/api/admin/users/1", body: map[string]bool{"disabled": true}, as: bob},
		{method: "GET", path: "/admin", as: bob},
	} {
		if w := h.do(r); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as a learner = %d, want 403", r.method, r.path, w.Code)
		}
	}
	if w := h.do(req{method: "GET", path: "/admin", as: admin}); w.Code != http.StatusOK {
		t.Errorf("GET /admin as admin = %d", w.Code)
	}
	if w := h.do(req{method: "GET", path: "/api/docs/pkg?path=strings", as: bob}); w.Code != http.StatusOK {
		t.Errorf("docs as a learner = %d", w.Code)
	}
}

func TestLoginLogoutAndDisable(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	bob := h.learner(admin, "bob")

	// A fresh login with the passkey.
	w := h.do(req{method: "POST", path: "/api/auth/login/begin", body: map[string]bool{"conditional": false}})
	wantStatus(t, w, http.StatusOK)
	begin := decodeJSON[struct {
		Ceremony string          `json:"ceremony"`
		Options  json.RawMessage `json:"options"`
	}](t, w)
	assertion, err := bob.key.Get(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/auth/login/finish", body: map[string]any{
		"ceremony": begin.Ceremony, "credential": assertion, "next": "https://evil.example/",
	}})
	wantStatus(t, w, http.StatusOK)
	if next := decodeJSON[map[string]any](t, w)["next"]; next != "/learn" {
		t.Fatalf("next = %v, want /learn for an off-site target", next)
	}
	again := h.userFrom(w, bob.key)
	wantStatus(t, h.do(req{method: "GET", path: "/learn", as: again}), http.StatusOK)

	wantStatus(t, h.do(req{method: "POST", path: "/api/auth/logout", body: map[string]any{}, as: again}), http.StatusOK)
	if w := h.do(req{method: "GET", path: "/learn", as: again}); w.Code != http.StatusSeeOther {
		t.Fatalf("after logout /learn = %d", w.Code)
	}

	// Disabling signs bob out at once.
	w = h.do(req{method: "PUT", path: "/api/admin/users/" + itoa(bob.ID), body: map[string]bool{"disabled": true}, as: admin})
	wantStatus(t, w, http.StatusOK)
	if w := h.do(req{method: "GET", path: "/api/exercises/" + logfmt, as: bob}); w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user API = %d", w.Code)
	}
	w = h.do(req{method: "PUT", path: "/api/admin/users/" + itoa(admin.ID), body: map[string]bool{"disabled": true}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestFilesSaveAndReset(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	w := h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/files/logfmt.go", body: map[string]string{"content": "package logfmt\n// mine\n"}, as: admin})
	wantStatus(t, w, http.StatusOK)
	for _, p := range []string{"logfmt_test.go", "..%2Fgo.mod", "nope.go"} {
		w := h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/files/" + p, body: map[string]string{"content": "x"}, as: admin})
		if w.Code != http.StatusNotFound {
			t.Errorf("save %s = %d, want 404", p, w.Code)
		}
	}
	big := strings.Repeat("x", runnerapi.MaxFileBytes+1)
	w = h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/files/logfmt.go", body: map[string]string{"content": big}, as: admin})
	wantStatus(t, w, http.StatusRequestEntityTooLarge)

	boot := decodeJSON[struct {
		Files []struct {
			Name     string `json:"name"`
			Kind     string `json:"kind"`
			ReadOnly bool   `json:"readonly"`
			Content  string `json:"content"`
			Saved    bool   `json:"saved"`
		} `json:"files"`
		Progress struct {
			Status string `json:"status"`
		} `json:"progress"`
		Steps []course.Step `json:"steps"`
	}](t, h.do(req{method: "GET", path: "/api/exercises/" + logfmt, as: admin}))
	if boot.Progress.Status != "started" {
		t.Fatalf("opening did not start the exercise: %+v", boot.Progress)
	}
	if len(boot.Steps) < 2 || boot.Steps[len(boot.Steps)-1].ID != "all" {
		t.Fatalf("steps = %+v", boot.Steps)
	}
	var sawSaved, sawTest bool
	for _, f := range boot.Files {
		if f.Name == "logfmt.go" {
			sawSaved = f.Saved && f.Content == "package logfmt\n// mine\n"
		}
		if f.Kind == "test" {
			sawTest = f.ReadOnly && f.Content != ""
		}
	}
	if !sawSaved || !sawTest {
		t.Fatalf("bootstrap files = %+v", boot.Files)
	}

	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/files/logfmt.go/reset", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusOK)
	ex, _ := testCourse.Exercise(logfmt)
	stub, _ := ex.File("logfmt.go")
	if got := decodeJSON[map[string]string](t, w)["content"]; got != stub.Content {
		t.Fatal("reset did not return the stub")
	}
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/99-nope", as: admin}), http.StatusNotFound)
}

func TestBootstrapHasNoHiddenMaterial(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	ex, _ := testCourse.Exercise(logfmt)
	hint, err := ex.Hidden(course.Hint1)
	if err != nil {
		t.Fatal(err)
	}
	var visible strings.Builder
	for _, f := range ex.Files {
		visible.WriteString(f.Content)
	}
	// A distinctive line of the solution and of hint 1 that no visible
	// file has.
	needles := []string{
		firstLongLine(ex.SolutionFiles()[0].Content, visible.String()),
		firstLongLine(stripTags(string(hint)), visible.String()),
	}
	for _, p := range []string{"/learn/" + logfmt, "/api/exercises/" + logfmt} {
		body := h.do(req{method: "GET", path: p, as: admin}).Body.String()
		for _, n := range needles {
			if strings.Contains(body, n) {
				t.Fatalf("%s contains hidden material %q", p, n)
			}
		}
	}
}

func firstLongLine(s, except string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); len(l) > 30 && !strings.HasPrefix(l, "//") && !strings.Contains(except, l) {
			return l
		}
	}
	return s
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestRevealOrderAndGates(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	reveal := func(level int, confirm bool) (int, map[string]any) {
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/reveal", body: map[string]any{"level": level, "confirm": confirm}, as: admin})
		return w.Code, decodeJSON[map[string]any](t, w)
	}

	if code, body := reveal(2, true); code != http.StatusConflict || body["code"] != "reveal_order" {
		t.Fatalf("level 2 first = %d %v", code, body)
	}
	if code, body := reveal(7, true); code != http.StatusBadRequest {
		t.Fatalf("level 7 = %d %v", code, body)
	}
	code, body := reveal(1, false)
	if code != http.StatusConflict || body["gate"] != "time" || body["kind"] != "active" || body["needed"] != 25.0 || body["minutes"] != 0.0 {
		t.Fatalf("hint 1 without time = %d %v", code, body)
	}
	code, body = reveal(1, true)
	if code != http.StatusOK || body["level"] != 1.0 || body["title"] != "Hint 1" || !strings.Contains(body["html"].(string), "<") {
		t.Fatalf("hint 1 confirmed = %d %v", code, body)
	}
	// Revealed levels show again without a gate.
	if code, _ := reveal(1, false); code != http.StatusOK {
		t.Fatalf("hint 1 again = %d", code)
	}
	h.clock.Advance(4 * time.Minute)
	code, body = reveal(2, false)
	if code != http.StatusConflict || body["kind"] != "since" || body["minutes"] != 4.0 || body["needed"] != 10.0 {
		t.Fatalf("hint 2 after 4 minutes = %d %v", code, body)
	}
	h.clock.Advance(6 * time.Minute)
	if code, body := reveal(2, false); code != http.StatusOK {
		t.Fatalf("hint 2 after 10 minutes = %d %v", code, body)
	}

	// 25 minutes of active time open hint 1 without a confirmation.
	second, third := testCourse.Exercises[1].Slug, testCourse.Exercises[2].Slug
	if err := h.store.AddActiveTime(t.Context(), admin.ID, second, 25*time.Minute); err != nil {
		t.Fatal(err)
	}
	w := h.do(req{method: "POST", path: "/api/exercises/" + second + "/reveal", body: map[string]any{"level": 1}, as: admin})
	wantStatus(t, w, http.StatusOK)

	// After a pass there are no gates, but the order stays.
	if _, err := h.store.MarkPassed(t.Context(), admin.ID, third); err != nil {
		t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/exercises/" + third + "/reveal", body: map[string]any{"level": 2}, as: admin})
	wantStatus(t, w, http.StatusConflict)
	for level := 1; level <= 6; level++ {
		w := h.do(req{method: "POST", path: "/api/exercises/" + third + "/reveal", body: map[string]any{"level": level}, as: admin})
		wantStatus(t, w, http.StatusOK)
	}

	boot := decodeJSON[struct {
		Revealed []struct {
			Level int    `json:"level"`
			HTML  string `json:"html"`
		} `json:"revealed"`
	}](t, h.do(req{method: "GET", path: "/api/exercises/" + logfmt, as: admin}))
	if len(boot.Revealed) != 2 || boot.Revealed[1].Level != 2 || boot.Revealed[1].HTML == "" {
		t.Fatalf("bootstrap revealed = %+v", boot.Revealed)
	}
}

func TestHeartbeatCredit(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	beat := func() int64 {
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/heartbeat", body: map[string]any{}, as: admin})
		wantStatus(t, w, http.StatusOK)
		return int64(decodeJSON[map[string]float64](t, w)["activeSeconds"])
	}
	if got := beat(); got != 15 {
		t.Fatalf("first beat = %d s, want 15", got)
	}
	h.clock.Advance(5 * time.Second)
	if got := beat(); got != 20 {
		t.Fatalf("a beat 5 s later = %d s, want 20", got)
	}
	h.clock.Advance(10 * time.Minute)
	if got := beat(); got != 35 {
		t.Fatalf("a beat after a break = %d s, want 35 (at most 15 s per beat)", got)
	}
}

func TestCheckExplainsDiagnostics(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{"files": map[string]string{"logfmt.go": "package logfmt\n"}}, as: admin})
	wantStatus(t, w, http.StatusOK)
	res := decodeJSON[struct {
		Diagnostics []struct {
			runnerapi.Diagnostic
			Explain struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				HTML  string `json:"html"`
				Raw   struct {
					File     string `json:"file"`
					Openable bool   `json:"openable"`
				} `json:"raw"`
			} `json:"explain"`
		} `json:"diagnostics"`
	}](t, w)
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Explain.ID == "" || res.Diagnostics[0].Explain.HTML == "" {
		t.Fatalf("check = %+v", res)
	}
	if raw := res.Diagnostics[0].Explain.Raw; raw.File != "logfmt.go" || !raw.Openable {
		t.Fatalf("explain raw = %+v, want logfmt.go and openable", raw)
	}
	// The runner got every stub: the sent one, and the others as stubs.
	ex, _ := testCourse.Exercise(logfmt)
	got := h.runner.checks[len(h.runner.checks)-1]
	if got.Exercise != logfmt || len(got.Files) != len(ex.StubFiles()) || got.Files["logfmt.go"] != "package logfmt\n" {
		t.Fatalf("runner request = %+v", got)
	}

	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{"files": map[string]string{"logfmt_test.go": "package logfmt"}}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)

	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/format", body: map[string]string{"file": "logfmt.go", "content": "package logfmt\n"}, as: admin})
	wantStatus(t, w, http.StatusOK)
	if got := decodeJSON[map[string]any](t, w)["content"]; got != "// formatted\npackage logfmt\n" {
		t.Fatalf("format = %v", got)
	}

	// The runner's queue is full: the learner sees 429 with Retry-After.
	h.runner.mu.Lock()
	h.runner.failWith = http.StatusTooManyRequests
	h.runner.mu.Unlock()
	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{"files": map[string]string{}}, as: admin})
	wantStatus(t, w, http.StatusTooManyRequests)
	if w.Header().Get("Retry-After") != "5" || decodeJSON[map[string]string](t, w)["code"] != "runner_busy" {
		t.Fatalf("busy runner = %v %s", w.Header(), w.Body.String())
	}

	// A runner that does not know an exercise of the site's course serves
	// another version of the course: web and runner roll out one after the
	// other. The learner hears that it is updating, not a refusal.
	h.runner.mu.Lock()
	h.runner.failWith, h.runner.failCode = http.StatusBadRequest, "unknown_exercise"
	h.runner.mu.Unlock()
	for _, r := range []req{
		{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{"files": map[string]string{}}},
		{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}},
	} {
		r.as = admin
		w = h.do(r)
		body := decodeJSON[map[string]string](t, w)
		if w.Code != http.StatusServiceUnavailable || body["code"] != "runner_updating" || !strings.Contains(body["error"], "updated") || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s with an older runner = %d %v %v", r.path, w.Code, w.Header(), body)
		}
	}
}

func TestRunnerDown(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	srv, err := newServerWithRunner(h, "http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	h.handler = srv
	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusServiceUnavailable)
	if code := decodeJSON[map[string]string](t, w)["code"]; code != "runner_unavailable" {
		t.Fatalf("code = %s", code)
	}
	// The site stays ready without the runner.
	wantStatus(t, h.do(req{method: "GET", path: "/readyz"}), http.StatusOK)
}

func TestTestStream(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.events = failingRun
	wantStatus(t, h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/files/logfmt.go", body: map[string]string{"content": "package logfmt\n// saved\n"}, as: admin}), http.StatusOK)

	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{"step": "step-1"}, as: admin})
	wantStatus(t, w, http.StatusOK)
	if ct := w.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("Content-Type = %q", ct)
	}
	events := ndjson(t, w.Body.String())
	types := make([]string, 0, len(events))
	for _, ev := range events {
		types = append(types, ev["type"].(string))
	}
	// start, the test events, explain right after the fail, done, progress.
	want := "start " + strings.Repeat("test ", len(failingRun(runnerapi.TestRequest{}))-1) + "explain done progress"
	if got := strings.Join(types, " "); got != want {
		t.Fatalf("event types:\n got %s\nwant %s", got, want)
	}
	explain := events[len(events)-3]
	items := explain["items"].([]any)
	if explain["test"] != "TestParse" || len(items) == 0 || items[0].(map[string]any)["id"] != "runtime-todo-implement" {
		t.Fatalf("explain event = %v", explain)
	}
	progress := events[len(events)-1]
	if progress["status"] != "started" || progress["passed"] != false || progress["allTests"] != false || progress["runs"] != 1.0 {
		t.Fatalf("progress = %v", progress)
	}

	// The runner got the saved file and the step's filter.
	ex, _ := testCourse.Exercise(logfmt)
	step, _ := ex.Step("step-1")
	got := h.runner.lastTest()
	if got.Files["logfmt.go"] != "package logfmt\n// saved\n" || got.Run != step.Run || got.Race != step.Race {
		t.Fatalf("runner request = %+v, want run %q", got, step.Run)
	}

	// A stub panic fails a subtest first and its parent after it. The
	// parent's specific explanation replaces the subtest's generic guide.
	h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
		return []runnerapi.Event{
			{Type: runnerapi.EventTest, Action: "run", Test: "TestParse"},
			{Type: runnerapi.EventTest, Action: "run", Test: "TestParse/one"},
			{Type: runnerapi.EventTest, Action: "output", Test: "TestParse/one", Output: "=== RUN   TestParse/one\n"},
			{Type: runnerapi.EventTest, Action: "output", Test: "TestParse/one", Output: "    --- FAIL: TestParse/one (0.00s)\n"},
			{Type: runnerapi.EventTest, Action: "fail", Test: "TestParse/one"},
			{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "--- FAIL: TestParse (0.00s)\n"},
			{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "panic: TODO: implement [recovered]\n"},
			{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "\tpanic: TODO: implement\n"},
			{Type: runnerapi.EventTest, Action: "fail", Test: "TestParse"},
			{Type: runnerapi.EventDone, ExitCode: 1, Failed: 1},
		}
	}
	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
	var explains []map[string]any
	for _, ev := range ndjson(t, w.Body.String()) {
		if ev["type"] == "explain" {
			explains = append(explains, ev)
		}
	}
	if len(explains) != 1 || explains[0]["test"] != "TestParse" {
		t.Fatalf("explain events = %v", explains)
	}
	if items := explains[0]["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != "runtime-todo-implement" {
		t.Fatalf("explain items = %v", items)
	}

	// A hand-written filter and an unknown step.
	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{"run": "TestX(", "race": true}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)
	w = h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{"step": "step-99"}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)
}

func TestProgressMarking(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	run := func(body map[string]any) map[string]any {
		t.Helper()
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: body, as: admin})
		wantStatus(t, w, http.StatusOK)
		events := ndjson(t, w.Body.String())
		last := events[len(events)-1]
		if last["type"] != "progress" {
			t.Fatalf("last event = %v", last)
		}
		return last
	}
	status := func() string {
		p, err := h.store.Progress(t.Context(), admin.ID, logfmt)
		if err != nil {
			return ""
		}
		return string(p.Status)
	}

	h.runner.events = passingRun
	if p := run(map[string]any{"step": "step-1", "race": true}); p["passed"] != true || p["allTests"] != false || status() == "passed" {
		t.Fatalf("a passing step passed the exercise: %v", p)
	}
	if p := run(map[string]any{"step": "all", "race": false}); p["allTests"] != false || status() == "passed" {
		t.Fatalf("a run without -race passed the exercise: %v", p)
	}
	h.runner.events = failingRun
	if p := run(map[string]any{"step": "all", "race": true}); p["passed"] != false || status() == "passed" {
		t.Fatalf("a failing run passed the exercise: %v", p)
	}
	h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
		return []runnerapi.Event{{Type: runnerapi.EventDone, ExitCode: -1, Passed: 3, TimedOut: true}}
	}
	if p := run(map[string]any{}); p["passed"] != false || status() == "passed" {
		t.Fatalf("a timed-out run passed the exercise: %v", p)
	}

	h.runner.events = passingRun
	p := run(map[string]any{"step": "all", "race": true})
	if p["status"] != "passed" || p["firstPass"] != true || p["allTests"] != true || p["passedAt"] == nil {
		t.Fatalf("all tests with -race = %v", p)
	}
	if got := h.runner.lastTest(); got.Run != "" || !got.Race {
		t.Fatalf("runner request = %+v", got)
	}
	p = run(map[string]any{})
	if p["status"] != "passed" || p["firstPass"] != false || p["runs"] != 6.0 {
		t.Fatalf("second pass = %v", p)
	}
	prog, _ := h.store.Progress(t.Context(), admin.ID, logfmt)
	var last map[string]any
	if err := json.Unmarshal(prog.LastResult, &last); err != nil || last["step"] != "all" || last["passed"] != 5.0 {
		t.Fatalf("last result = %s", prog.LastResult)
	}
	// The IDE shows the last run's tree after a reload.
	if tests, _ := last["tests"].(map[string]any); tests["TestParse"] != "pass" {
		t.Fatalf("last result tests = %v", last["tests"])
	}
	h.runner.events = failingRun
	run(map[string]any{"step": "all", "race": true})
	prog, _ = h.store.Progress(t.Context(), admin.ID, logfmt)
	last = nil
	if err := json.Unmarshal(prog.LastResult, &last); err != nil {
		t.Fatal(err)
	}
	if tests, _ := last["tests"].(map[string]any); tests["TestParse"] != "fail" {
		t.Fatalf("last result tests after a failure = %v", last["tests"])
	}
}

func TestOneRunAtATime(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.block = make(chan struct{})
	h.runner.started = make(chan struct{}, 1)
	done := make(chan int, 1)
	go func() {
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
		done <- w.Code
	}()
	select {
	case <-h.runner.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first run did not start")
	}
	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusConflict)
	close(h.runner.block)
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("first run = %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the first run did not end")
	}
	h.runner.block = nil
	h.runner.started = nil
	wantStatus(t, h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin}), http.StatusOK)
}

func TestTestRateLimit(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	for i := range 10 {
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
		if w.Code != http.StatusOK {
			t.Fatalf("run %d = %d", i, w.Code)
		}
	}
	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusTooManyRequests)
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
}
