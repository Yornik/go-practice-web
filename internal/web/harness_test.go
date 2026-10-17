package web_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/auth/authtest"
	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/runnerapi"
	"github.com/yornik/go-practice-web/internal/store"
	"github.com/yornik/go-practice-web/internal/web"
)

const (
	publicURL   = "https://go-learn.party"
	runnerToken = "test-runner-token-0123456789abcdef"
)

// testCourse is loaded once: Load renders every README.
var testCourse *course.Course

func TestMain(m *testing.M) {
	c, err := course.Load(filepath.Join("..", "..", "course"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "load course:", err)
		os.Exit(1)
	}
	testCourse = c
	os.Exit(m.Run())
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// mailbox keeps every message. It does not claim delivery, like the log
// sender, until a test calls setDelivers: then it stands in for SMTP.
type mailbox struct {
	mu       sync.Mutex
	msgs     []mail.Message
	delivers bool
}

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	return nil
}

func (m *mailbox) Delivers() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.delivers
}

func (m *mailbox) setDelivers(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delivers = v
}

// fakeRunner answers the runner API with canned responses and records the
// requests.
type fakeRunner struct {
	mu       sync.Mutex
	checks   []runnerapi.CheckRequest
	tests    []runnerapi.TestRequest
	events   func(runnerapi.TestRequest) []runnerapi.Event
	block    chan struct{} // when set, a test run waits for it to close
	started  chan struct{} // receives when a test run starts
	failWith int           // when set, every call answers this status

	checkStarted chan struct{} // receives when a check starts
	checkBlock   chan struct{} // when set, a check waits for it to close
	checkGone    chan struct{} // receives when a waiting check's caller hung up

	info       *runnerapi.Info // the answer of GET /v1/info; nil answers 503
	infoCalls  int
	infoStatus int // when set, GET /v1/info answers this status
}

func (f *fakeRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+runnerToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	failWith := f.failWith
	f.mu.Unlock()
	if failWith != 0 {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(failWith)
		_ = json.NewEncoder(w).Encode(runnerapi.Error{Error: "queue is full", Code: "queue_full"})
		return
	}
	switch r.URL.Path {
	case runnerapi.PathInfo:
		f.mu.Lock()
		f.infoCalls++
		info, status := f.info, f.infoStatus
		f.mu.Unlock()
		if status == 0 && info == nil {
			status = http.StatusServiceUnavailable
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(runnerapi.Error{Error: "not ready", Code: "not_ready"})
			return
		}
		_ = json.NewEncoder(w).Encode(info)
	case runnerapi.PathCheck:
		var req runnerapi.CheckRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.checks = append(f.checks, req)
		started, block, gone := f.checkStarted, f.checkBlock, f.checkGone
		f.mu.Unlock()
		if started != nil {
			started <- struct{}{}
		}
		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				if gone != nil {
					gone <- struct{}{}
				}
				return
			}
		}
		_ = json.NewEncoder(w).Encode(runnerapi.CheckResponse{Diagnostics: []runnerapi.Diagnostic{{
			File: "logfmt.go", Line: 3, Column: 2, Severity: runnerapi.SeverityError,
			Source: runnerapi.SourceCompile, Message: "undefined: foo",
		}}, DurationMs: 12})
	case runnerapi.PathFormat:
		var req runnerapi.FormatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(runnerapi.FormatResponse{Content: "// formatted\n" + req.Content})
	case runnerapi.PathTest:
		var req runnerapi.TestRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.tests = append(f.tests, req)
		events, block, started := f.events, f.block, f.started
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		_ = enc.Encode(runnerapi.Event{Type: runnerapi.EventStart})
		w.(http.Flusher).Flush()
		if started != nil {
			started <- struct{}{}
		}
		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				return
			}
		}
		if events == nil {
			events = passingRun
		}
		for _, ev := range events(req) {
			_ = enc.Encode(ev)
		}
	default:
		http.NotFound(w, r)
	}
}

// setInfo sets the answer of GET /v1/info and returns how many times it
// was asked before.
func (f *fakeRunner) setInfo(info *runnerapi.Info, status int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info, f.infoStatus = info, status
	return f.infoCalls
}

func (f *fakeRunner) infoCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.infoCalls
}

func (f *fakeRunner) lastTest() runnerapi.TestRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tests[len(f.tests)-1]
}

// passingRun passes every top-level test of the exercise.
func passingRun(req runnerapi.TestRequest) []runnerapi.Event {
	events := []runnerapi.Event{
		{Type: runnerapi.EventTest, Action: "run", Test: "TestParse"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "=== RUN   TestParse\n"},
		{Type: runnerapi.EventTest, Action: "pass", Test: "TestParse", Elapsed: 0.01},
	}
	if ex, ok := testCourse.Exercise(req.Exercise); ok {
		for _, name := range ex.Tests {
			if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Fuzz") {
				events = append(events, runnerapi.Event{Type: runnerapi.EventTest, Action: "pass", Test: name})
			}
		}
	}
	return append(events, runnerapi.Event{Type: runnerapi.EventDone, ExitCode: 0, Passed: 5, DurationMs: 900})
}

func failingRun(runnerapi.TestRequest) []runnerapi.Event {
	return []runnerapi.Event{
		{Type: runnerapi.EventTest, Action: "run", Test: "TestParse"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "=== RUN   TestParse\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "--- FAIL: TestParse (0.00s)\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "panic: TODO: implement [recovered]\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "\tpanic: TODO: implement\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "\ngoroutine 7 [running]:\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "example.com/exercises/01-logfmt.Parse(...)\n"},
		{Type: runnerapi.EventTest, Action: "output", Test: "TestParse", Output: "\texercises/01-logfmt/logfmt.go:30 +0x25\n"},
		{Type: runnerapi.EventTest, Action: "fail", Test: "TestParse", Elapsed: 0.01},
		{Type: runnerapi.EventDone, ExitCode: 1, Failed: 1, DurationMs: 800},
	}
}

// harness is a web server with a real store, real auth and a fake runner.
type harness struct {
	t       *testing.T
	cfg     web.Config
	srv     *web.Server
	handler http.Handler
	store   *store.Store
	auth    *auth.Service
	runner  *fakeRunner
	clock   *clock
	mails   *mailbox
	signer  *cert.Signer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clk := &clock{t: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "web.db"), store.WithClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mb := &mailbox{}
	a, err := auth.New(st, mail.NewMailer(mb, publicURL), auth.Config{PublicURL: publicURL, Now: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Wait)
	fake := &fakeRunner{info: &runnerapi.Info{
		GoVersion: "go1.27.2", RaceAvailable: true, Staticcheck: true,
		Sandbox: "landlock ABI 6: files, ptrace, tcp, signals", LandlockABI: 6,
	}}
	rs := httptest.NewServer(fake)
	t.Cleanup(rs.Close)
	seed := sha256.Sum256([]byte("test signing key"))
	signer, err := cert.NewSigner(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	cfg := web.Config{
		PublicURL: publicURL,
		Store:     st,
		Course:    testCourse,
		Auth:      a,
		Runner:    web.NewRunnerClient(rs.URL, runnerToken, nil),
		Signer:    signer,
		Templates: os.DirFS(filepath.Join("..", "..", "web", "templates")),
		Static:    os.DirFS(filepath.Join("..", "..", "web", "static")),
		Docs:      http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"docs":true}`)) }),
		Now:       clk.Now,
	}
	srv, err := web.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, cfg: cfg, srv: srv, handler: srv.Handler(), store: st, auth: a, runner: fake, clock: clk, mails: mb, signer: signer}
}

// newServerWithRunner builds the harness's server again with another
// runner URL.
func newServerWithRunner(h *harness, runnerURL string) (http.Handler, error) {
	cfg := h.cfg
	cfg.Runner = web.NewRunnerClient(runnerURL, runnerToken, nil)
	srv, err := web.New(cfg)
	if err != nil {
		return nil, err
	}
	return srv.Handler(), nil
}

// user is a signed-in browser.
type user struct {
	store.User
	cookie *http.Cookie
	key    *authtest.Authenticator
}

type req struct {
	method string
	path   string
	body   any
	as     *user
	header map[string]string // overrides; "" deletes
}

// do sends a request like a browser on the site: with the CSRF header and
// the site's Origin, unless the test overrides them.
func (h *harness) do(r req) *httptest.ResponseRecorder {
	h.t.Helper()
	var body io.Reader
	if r.body != nil {
		b, err := json.Marshal(r.body)
		if err != nil {
			h.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	hr := httptest.NewRequestWithContext(h.t.Context(), r.method, publicURL+r.path, body)
	hr.RemoteAddr = "192.0.2.1:1234"
	if r.method != http.MethodGet {
		hr.Header.Set("X-GPW-CSRF", "1")
		hr.Header.Set("Origin", publicURL)
		hr.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.header {
		if v == "" {
			hr.Header.Del(k)
		} else {
			hr.Header.Set(k, v)
		}
	}
	if r.as != nil {
		hr.AddCookie(r.as.cookie)
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, hr)
	return w
}

func decodeJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

func wantStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d; body %s", w.Code, status, w.Body.String())
	}
}

// signUp registers through the API with a software authenticator.
func (h *harness) signUp(body map[string]string) *user {
	h.t.Helper()
	u, _ := h.signUpNext(body)
	return u
}

// signUpNext registers like signUp and also returns where the site sends
// the new user.
func (h *harness) signUpNext(body map[string]string) (*user, string) {
	h.t.Helper()
	key := authtest.New(publicURL)
	w := h.do(req{method: "POST", path: "/api/auth/register/begin", body: body})
	wantStatus(h.t, w, http.StatusOK)
	begin := decodeJSON[struct {
		Ceremony string          `json:"ceremony"`
		Options  json.RawMessage `json:"options"`
	}](h.t, w)
	cred, _, err := key.Create(begin.Options)
	if err != nil {
		h.t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/auth/register/finish", body: map[string]any{"ceremony": begin.Ceremony, "credential": cred}})
	wantStatus(h.t, w, http.StatusOK)
	next := decodeJSON[struct {
		Next string `json:"next"`
	}](h.t, w).Next
	return h.userFrom(w, key), next
}

func (h *harness) userFrom(w *httptest.ResponseRecorder, key *authtest.Authenticator) *user {
	h.t.Helper()
	res := decodeJSON[struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}](h.t, w)
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "__Host-gpw_session" {
			cookie = c
		}
	}
	if cookie == nil {
		h.t.Fatalf("no session cookie in %v", w.Result().Cookies())
	}
	u, err := h.store.User(h.t.Context(), res.User.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return &user{User: u, cookie: cookie, key: key}
}

// admin signs up the first account with the setup link and verifies its
// address, so it passes the email gate.
func (h *harness) admin() *user {
	h.t.Helper()
	return h.verify(h.unverifiedAdmin())
}

// unverifiedAdmin signs up the first account and leaves the verification
// link unused: the account is behind the email gate.
func (h *harness) unverifiedAdmin() *user {
	h.t.Helper()
	link, err := h.auth.EnsureSetupLink(h.t.Context())
	if err != nil {
		h.t.Fatal(err)
	}
	u, _ := url.Parse(link)
	return h.signUp(map[string]string{"setup": u.Query().Get("token"), "username": "ada", "displayName": "Ada Admin", "email": "ada@example.com"})
}

// learner joins with an invite the admin creates, as <username>@example.com,
// and verifies the address.
func (h *harness) learner(admin *user, username string) *user {
	h.t.Helper()
	return h.verify(h.unverifiedLearner(admin, username))
}

// unverifiedLearner joins with a copied invite link and leaves the
// verification link unused.
func (h *harness) unverifiedLearner(admin *user, username string) *user {
	h.t.Helper()
	w := h.do(req{method: "POST", path: "/api/admin/invites", body: map[string]any{"days": 7}, as: admin})
	wantStatus(h.t, w, http.StatusCreated)
	inv := decodeJSON[struct {
		URL string `json:"url"`
	}](h.t, w)
	u, _ := url.Parse(inv.URL)
	return h.signUp(map[string]string{
		"invite": u.Query().Get("invite"), "username": username,
		"displayName": strings.ToUpper(username[:1]) + username[1:], "email": username + "@example.com",
	})
}

// verify uses the newest verification link mailed to u's address, as the
// verify page does, and refreshes u from the store.
func (h *harness) verify(u *user) *user {
	h.t.Helper()
	token := h.lastLink(u.Email, "Verify")
	wantStatus(h.t, h.do(req{method: "POST", path: "/api/auth/verify-email", body: map[string]string{"token": token}}), http.StatusOK)
	fresh, err := h.store.User(h.t.Context(), u.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	if !fresh.EmailVerified() {
		h.t.Fatalf("%s is not verified after the link", u.Username)
	}
	u.User = fresh
	return u
}

// ndjson splits a stream body into events.
func ndjson(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}
