package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// TestEveryMutationNeedsCSRF sends every state-changing route of
// docs/DESIGN.md without the CSRF header and from a foreign origin. The
// check is global middleware; this keeps a route from slipping past it if
// that ever changes.
func TestEveryMutationNeedsCSRF(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	learner := h.learner(admin, "grace")
	routes := []struct{ method, path string }{
		{"POST", "/api/auth/register/begin"},
		{"POST", "/api/auth/register/finish"},
		{"POST", "/api/auth/login/begin"},
		{"POST", "/api/auth/login/finish"},
		{"POST", "/api/auth/logout"},
		{"POST", "/api/auth/passkeys/begin"},
		{"POST", "/api/auth/passkeys/finish"},
		{"PUT", "/api/auth/passkeys/AAAA"},
		{"DELETE", "/api/auth/passkeys/AAAA"},
		{"POST", "/api/auth/recover"},
		{"POST", "/api/auth/recover/begin"},
		{"POST", "/api/auth/recover/finish"},
		{"POST", "/api/auth/verify-email"},
		{"POST", "/api/auth/verify-email/resend"},
		{"PUT", "/api/account"},
		{"PUT", "/api/exercises/" + logfmt + "/files/logfmt.go"},
		{"POST", "/api/exercises/" + logfmt + "/files/logfmt.go/reset"},
		{"POST", "/api/exercises/" + logfmt + "/check"},
		{"POST", "/api/exercises/" + logfmt + "/format"},
		{"POST", "/api/exercises/" + logfmt + "/test"},
		{"POST", "/api/exercises/" + logfmt + "/reveal"},
		{"POST", "/api/exercises/" + logfmt + "/heartbeat"},
		{"PUT", "/api/exercises/" + logfmt + "/notes"},
		{"POST", "/api/certificates"},
		{"POST", "/api/admin/invites"},
		{"DELETE", "/api/admin/invites/" + "00"},
		{"PUT", "/api/admin/users/" + strconv.FormatInt(learner.ID, 10)},
		// Methods the site does not use are refused before routing.
		{"PATCH", "/api/account"},
		{"POST", "/learn"},
	}
	refusals := []struct {
		name   string
		header map[string]string
		code   string
	}{
		{"no CSRF header", map[string]string{"X-GPW-CSRF": ""}, "csrf"},
		{"foreign origin", map[string]string{"Origin": "https://evil.example"}, "origin"},
		{"null origin", map[string]string{"Origin": "null"}, "origin"},
	}
	for _, rt := range routes {
		for _, ref := range refusals {
			w := h.do(req{method: rt.method, path: rt.path, body: map[string]any{}, as: admin, header: ref.header})
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s with %s: status %d, want 403", rt.method, rt.path, ref.name, w.Code)
				continue
			}
			if got := decodeJSON[map[string]string](t, w)["code"]; got != ref.code {
				t.Errorf("%s %s with %s: code %q, want %q", rt.method, rt.path, ref.name, got, ref.code)
			}
		}
	}
	// Nothing went through: the session still works and the learner is
	// still enabled.
	wantStatus(t, h.do(req{method: "GET", path: "/api/certificates", as: admin}), http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/api/certificates", as: learner}), http.StatusOK)
}

func TestReadyzCachesTheProbe(t *testing.T) {
	h := newHarness(t)
	// A client that hangs up must not leave a failure in the cache for
	// the kubelet's next probe.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, publicURL+"/readyz", nil)
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	wantStatus(t, w, http.StatusOK)

	// Within the window the result is reused: no write reaches the
	// database, which is closed now.
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		wantStatus(t, h.do(req{method: "GET", path: "/readyz"}), http.StatusOK)
	}
	// After it, the probe runs again and sees the closed database.
	h.clock.Advance(3 * time.Second)
	wantStatus(t, h.do(req{method: "GET", path: "/readyz"}), http.StatusServiceUnavailable)
	if cc := h.do(req{method: "GET", path: "/readyz"}).Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestOneCheckInFlightPerUser(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	grace := h.learner(admin, "grace")
	started, gone, block := make(chan struct{}, 4), make(chan struct{}, 4), make(chan struct{})
	h.runner.mu.Lock()
	h.runner.checkStarted, h.runner.checkGone, h.runner.checkBlock = started, gone, block
	h.runner.mu.Unlock()
	check := func(u *user) <-chan *httptest.ResponseRecorder {
		ch := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			ch <- h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/check", body: map[string]any{}, as: u})
		}()
		return ch
	}
	wait := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}

	first := check(admin)
	wait(started, "the first check")
	other := check(grace)
	wait(started, "another user's check")
	second := check(admin)
	wait(started, "the second check")

	// The newer check replaces the older one of the same user, at the
	// runner too, so 1 user holds at most 1 runner slot for checks.
	select {
	case w := <-first:
		wantStatus(t, w, http.StatusConflict)
		if got := decodeJSON[map[string]string](t, w)["code"]; got != "superseded" {
			t.Errorf("code = %q, want superseded", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the first check was not cancelled")
	}
	wait(gone, "the runner to see the first check go")
	close(block)
	for name, ch := range map[string]<-chan *httptest.ResponseRecorder{"second": second, "other user's": other} {
		select {
		case w := <-ch:
			if w.Code != http.StatusOK {
				t.Errorf("%s check: status %d, want 200; %s", name, w.Code, w.Body.String())
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s check did not finish", name)
		}
	}
}

func TestWritesAreRateLimited(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	grace := h.learner(admin, "grace")
	// Heartbeats, saves and notes share 1 budget per user.
	for i := range 120 {
		w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/heartbeat", as: admin})
		if w.Code != http.StatusOK {
			t.Fatalf("heartbeat %d: status %d", i, w.Code)
		}
	}
	for _, r := range []req{
		{method: "POST", path: "/api/exercises/" + logfmt + "/heartbeat", as: admin},
		{method: "PUT", path: "/api/exercises/" + logfmt + "/files/logfmt.go", body: map[string]string{"content": "package logfmt\n"}, as: admin},
		{method: "POST", path: "/api/exercises/" + logfmt + "/files/logfmt.go/reset", as: admin},
		{method: "PUT", path: "/api/exercises/" + logfmt + "/notes", body: map[string]string{"notes": "x"}, as: admin},
	} {
		w := h.do(r)
		wantStatus(t, w, http.StatusTooManyRequests)
		if w.Header().Get("Retry-After") == "" {
			t.Errorf("%s %s: no Retry-After", r.method, r.path)
		}
	}
	// Other users have their own budget, and it refills.
	wantStatus(t, h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/notes", body: map[string]string{"notes": "x"}, as: grace}), http.StatusOK)
	h.clock.Advance(time.Minute)
	wantStatus(t, h.do(req{method: "PUT", path: "/api/exercises/" + logfmt + "/notes", body: map[string]string{"notes": "x"}, as: admin}), http.StatusOK)
}

func TestCertificateClaimsAreRateLimited(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	tier1, _ := testCourse.Tier(1)
	for _, e := range tier1.Exercises {
		if _, err := h.store.MarkPassed(t.Context(), admin.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 10 {
		w := h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "tier1"}, as: admin})
		if w.Code != http.StatusCreated {
			t.Fatalf("claim %d: status %d; %s", i, w.Code, w.Body.String())
		}
	}
	w := h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "tier1"}, as: admin})
	wantStatus(t, w, http.StatusTooManyRequests)
}

func TestPDFLimitCountsIPv6PerPrefix(t *testing.T) {
	h := newHarness(t)
	get := func(remote string) int {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, publicURL+"/verify/GP-0000-0000-0000.pdf", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		return w.Code
	}
	// Every request comes from another address of the same /64.
	for i := range 30 {
		if code := get("[2001:db8:1:2::" + strconv.FormatInt(int64(i+1), 16) + "]:443"); code != http.StatusNotFound {
			t.Fatalf("request %d: status %d, want 404", i, code)
		}
	}
	if code := get("[2001:db8:1:2:ffff::1]:443"); code != http.StatusTooManyRequests {
		t.Errorf("31st request from the /64: status %d, want 429", code)
	}
	if code := get("[2001:db8:1:3::1]:443"); code != http.StatusNotFound {
		t.Errorf("request from another /64: status %d, want 404", code)
	}
}

// TestFakedPassDoesNotPassTheExercise sends what learner code can print
// from an init function: test2json lines for 1 test, then os.Exit(0). The
// runner then reports a clean done event; the exercise must not pass.
func TestFakedPassDoesNotPassTheExercise(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	h.runner.mu.Lock()
	h.runner.events = func(runnerapi.TestRequest) []runnerapi.Event {
		return []runnerapi.Event{
			{Type: runnerapi.EventTest, Action: "run", Test: "TestParse"},
			{Type: runnerapi.EventTest, Action: "pass", Test: "TestParse"},
			{Type: runnerapi.EventTest, Action: "pass"},
			{Type: runnerapi.EventDone, ExitCode: 0, Passed: 1},
		}
	}
	h.runner.mu.Unlock()
	w := h.do(req{method: "POST", path: "/api/exercises/" + logfmt + "/test", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusOK)
	events := ndjson(t, w.Body.String())
	if last := events[len(events)-1]; last["type"] != "progress" || last["passed"] != false || last["status"] == "passed" {
		t.Fatalf("progress = %v, want not passed", last)
	}
	if p, err := h.store.Progress(t.Context(), admin.ID, logfmt); err == nil && p.Passed() {
		t.Fatal("the exercise passed")
	}
}
