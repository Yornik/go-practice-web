package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
)

// ungated lists every route that skips the email gate, with the access it
// has instead. A new route is gated unless it is added here, so a course,
// certificate or admin route cannot slip past the gate by accident.
var ungated = map[string]string{
	// Public pages and probes.
	"GET /{$}":             "public",
	"GET /login":           "public",
	"GET /join":            "public",
	"GET /setup":           "public",
	"GET /recover":         "public",
	"GET /recover/confirm": "public",
	"GET /verify-email":    "public",
	"GET /guide":           "public",
	"GET /verify":          "public",
	"GET /verify/{id}":     "public",
	"GET /healthz":         "public",
	"GET /readyz":          "public",
	"GET /static/":         "public",
	"GET /highlight.css":   "public",
	"GET /favicon.ico":     "public",
	"/":                    "public",
	"/api/":                "public",
	"GET /.well-known/go-practice-certificates.pub": "public",
	// Signing up, in and back in, and using a verification link.
	"POST /api/auth/register/begin":  "public",
	"POST /api/auth/register/finish": "public",
	"POST /api/auth/login/begin":     "public",
	"POST /api/auth/login/finish":    "public",
	"POST /api/auth/recover":         "public",
	"POST /api/auth/recover/begin":   "public",
	"POST /api/auth/recover/finish":  "public",
	"POST /api/auth/verify-email":    "public",
	// A user with an unverified address can fix it, ask for a new link,
	// keep the passkeys in order and sign out.
	"GET /check-email":                   "user",
	"GET /account":                       "user",
	"PUT /api/account":                   "user",
	"POST /api/auth/verify-email/resend": "user",
	"POST /api/auth/passkeys/begin":      "user",
	"POST /api/auth/passkeys/finish":     "user",
	"PUT /api/auth/passkeys/{id}":        "user",
	"DELETE /api/auth/passkeys/{id}":     "user",
	"POST /api/auth/logout":              "user",
}

// examplePaths fills in the wildcards of a pattern with values that reach
// the handler.
var examplePaths = map[string]string{
	"GET /{$}":       "/",
	"GET /static/":   "/static/css/site.css",
	"GET /api/docs/": "/api/docs/pkg?path=strings",
	"/":              "/nope",
	"/api/":          "/api/nope",
}

// request builds a request for a route pattern as user u.
func routeRequest(pattern string, u *user) req {
	method, p, ok := strings.Cut(pattern, " ")
	if !ok {
		method, p = http.MethodGet, pattern
	}
	if ex, ok := examplePaths[pattern]; ok {
		p = ex
	}
	p = strings.NewReplacer(
		"{exercise}", "01-logfmt", "{file}", "logfmt.go", "{hash}", "00",
	).Replace(p)
	switch {
	case strings.HasPrefix(p, "/api/admin/users/"):
		p = strings.Replace(p, "{id}", itoa(u.ID), 1)
	case strings.HasPrefix(p, "/verify/"):
		p = strings.Replace(p, "{id}", "GP-0000-0000-0000", 1)
	default:
		p = strings.Replace(p, "{id}", "AAAA", 1)
	}
	r := req{method: method, path: p, as: u}
	if method != http.MethodGet {
		r.body = map[string]any{}
	}
	return r
}

// gated reports whether the email gate answered: a redirect to
// /check-email for a page, 403 email_unverified for an API.
func gated(w *httptest.ResponseRecorder) bool {
	if w.Code == http.StatusSeeOther && w.Header().Get("Location") == "/check-email" {
		return true
	}
	return w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), `"code":"email_unverified"`)
}

// TestEveryRouteIsGatedOrAllowed walks the server's own route table: each
// route is behind the email gate, or on the allow-list above with the
// access it has. Then it sends every route as a user with an unverified
// address, and again after the address is verified.
func TestEveryRouteIsGatedOrAllowed(t *testing.T) {
	h := newHarness(t)
	ada := h.unverifiedAdmin()
	routes := h.srv.Routes()
	seen := map[string]bool{}
	for _, rt := range routes {
		seen[rt.Pattern] = true
		want, allowed := ungated[rt.Pattern]
		switch {
		case !allowed && rt.Access != "verified" && rt.Access != "admin":
			t.Errorf("%s skips the email gate (access %s): register it with coursePage, courseAPI, adminPage or adminAPI, or add it to ungated", rt.Pattern, rt.Access)
		case allowed && rt.Access != want:
			t.Errorf("%s has access %s, the allow-list says %s", rt.Pattern, rt.Access, want)
		}
	}
	for p := range ungated {
		if !seen[p] {
			t.Errorf("ungated lists %s, which is not a route", p)
		}
	}

	// Behind the gate: every gated route turns the unverified user away.
	var behind []string
	for _, rt := range routes {
		if _, ok := ungated[rt.Pattern]; ok {
			continue
		}
		behind = append(behind, rt.Pattern)
		r := routeRequest(rt.Pattern, ada)
		w := h.do(r)
		if strings.HasPrefix(r.path, "/api/") {
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s unverified: status %d, want 403; %s", r.method, r.path, w.Code, w.Body.String())
				continue
			}
			body := decodeJSON[map[string]string](t, w)
			if body["code"] != "email_unverified" || !strings.Contains(body["error"], "Verify your email address") {
				t.Errorf("%s %s unverified: body %v", r.method, r.path, body)
			}
		} else if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/check-email" {
			t.Errorf("%s %s unverified: %d to %q, want 303 to /check-email", r.method, r.path, w.Code, w.Header().Get("Location"))
		}
	}
	if len(behind) < 15 {
		t.Fatalf("only %d gated routes: %v", len(behind), behind)
	}

	// The allow-list stays open to the unverified user. Sign-out comes last.
	for _, rt := range routes {
		if _, ok := ungated[rt.Pattern]; !ok || rt.Pattern == "POST /api/auth/logout" {
			continue
		}
		r := routeRequest(rt.Pattern, ada)
		if w := h.do(r); gated(w) {
			t.Errorf("%s %s: the gate answered an allowed route: %d %s", r.method, r.path, w.Code, w.Body.String())
		}
	}

	// Once the address is verified, the gate opens for every route.
	ada = h.verify(ada)
	for _, p := range behind {
		r := routeRequest(p, ada)
		if w := h.do(r); gated(w) {
			t.Errorf("%s %s verified: the gate still answers: %d %s", r.method, r.path, w.Code, w.Body.String())
		}
	}
	w := h.do(routeRequest("POST /api/auth/logout", ada))
	wantStatus(t, w, http.StatusOK)
}

// TestEmailGate is the gate as docs/DESIGN.md lists it.
func TestEmailGate(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	grace := h.unverifiedLearner(admin, "grace")
	for _, p := range []string{"/learn", "/learn/01-logfmt", "/learn/99-nope", "/certificates", "/admin"} {
		w := h.do(req{method: "GET", path: p, as: grace})
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/check-email" {
			t.Errorf("GET %s = %d to %q, want 303 to /check-email", p, w.Code, w.Header().Get("Location"))
		}
	}
	for _, r := range []req{
		{method: "GET", path: "/api/exercises/01-logfmt"},
		{method: "PUT", path: "/api/exercises/01-logfmt/files/logfmt.go", body: map[string]string{"content": "package logfmt\n"}},
		{method: "POST", path: "/api/exercises/01-logfmt/check", body: map[string]any{}},
		{method: "POST", path: "/api/exercises/01-logfmt/test", body: map[string]any{}},
		{method: "POST", path: "/api/exercises/01-logfmt/reveal", body: map[string]int{"level": 1}},
		{method: "POST", path: "/api/exercises/01-logfmt/heartbeat", body: map[string]any{}},
		{method: "GET", path: "/api/docs/pkg?path=strings"},
		{method: "GET", path: "/api/certificates"},
		{method: "POST", path: "/api/certificates", body: map[string]string{"kind": "tier1"}},
		{method: "POST", path: "/api/admin/invites", body: map[string]any{}},
		{method: "PUT", path: "/api/admin/users/" + itoa(admin.ID), body: map[string]bool{"admin": false}},
	} {
		r.as = grace
		w := h.do(r)
		wantStatus(t, w, http.StatusForbidden)
		if got := decodeJSON[map[string]string](t, w)["code"]; got != "email_unverified" {
			t.Errorf("%s %s: code %q, want email_unverified", r.method, r.path, got)
		}
	}
	// Nothing went through: no runner call, no saved code, no progress.
	h.runner.mu.Lock()
	checks, runs := len(h.runner.checks), len(h.runner.tests)
	h.runner.mu.Unlock()
	if checks != 0 || runs != 0 {
		t.Errorf("the runner saw %d checks and %d runs", checks, runs)
	}
	if code, err := h.store.Code(t.Context(), grace.ID, "01-logfmt"); err != nil || len(code) != 0 {
		t.Errorf("saved code = %v, %v", code, err)
	}
	if p, err := h.store.UserProgress(t.Context(), grace.ID); err != nil || len(p) != 0 {
		t.Errorf("progress = %v, %v", p, err)
	}

	// The way out stays open: the account page, a new link, sign-out.
	wantStatus(t, h.do(req{method: "GET", path: "/account", as: grace}), http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/check-email", as: grace}), http.StatusOK)
	wantStatus(t, h.do(req{method: "POST", path: "/api/auth/verify-email/resend", body: map[string]any{}, as: grace}), http.StatusOK)

	// A login lands on /learn as always; the gate sends it on.
	w := h.do(req{method: "POST", path: "/api/auth/login/begin", body: map[string]bool{"conditional": false}})
	wantStatus(t, w, http.StatusOK)
	begin := decodeJSON[struct {
		Ceremony string          `json:"ceremony"`
		Options  json.RawMessage `json:"options"`
	}](t, w)
	assertion, err := grace.key.Get(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/auth/login/finish", body: map[string]any{"ceremony": begin.Ceremony, "credential": assertion}})
	wantStatus(t, w, http.StatusOK)
	if next := decodeJSON[map[string]any](t, w)["next"]; next != "/learn" {
		t.Errorf("login next = %v, want /learn", next)
	}
	again := h.userFrom(w, grace.key)
	if w := h.do(req{method: "GET", path: "/learn", as: again}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/check-email" {
		t.Errorf("/learn after login = %d to %q", w.Code, w.Header().Get("Location"))
	}

	// The link opens the gate.
	grace = h.verify(grace)
	wantStatus(t, h.do(req{method: "GET", path: "/learn", as: grace}), http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/01-logfmt", as: grace}), http.StatusOK)
	// A verified user changing the address keeps the old one until the new
	// one is verified, so the gate stays open.
	w = h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Grace", "email": "grace@example.org"}, as: grace})
	wantStatus(t, w, http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/learn", as: grace}), http.StatusOK)
}

func TestCheckEmailPage(t *testing.T) {
	h := newHarness(t)
	w := h.do(req{method: "GET", path: "/check-email"})
	login := "/login?" + url.Values{"next": {"/check-email"}}.Encode()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != login {
		t.Fatalf("anonymous /check-email = %d to %q", w.Code, w.Header().Get("Location"))
	}

	ada := h.unverifiedAdmin()
	page := func() string {
		t.Helper()
		w := h.do(req{method: "GET", path: "/check-email", as: ada})
		wantStatus(t, w, http.StatusOK)
		return w.Body.String()
	}
	body := page()
	for _, want := range []string{
		"<title>Check your email, Go Practice</title>",
		"We sent a link to <strong>ada@example.com</strong>. Open it to start the course.",
		"The link works for 48 hours.",
		">Send the link again</button>",
		`<a class="btn" href="/account">Change the address</a>`,
		`data-logout>Sign out</button>`,
		"pages/check-email.js",
		// The harness's mailbox is like the log sender.
		"<b>NOTE:</b> This server sends no email: SMTP is not set up. The link is in the server log",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// With a mail server, the operator's note is gone.
	h.mails.setDelivers(true)
	if body := page(); strings.Contains(body, "server log") {
		t.Error("the note about the server log shows with SMTP")
	}

	// An expired link says so; a new one is on its way at once.
	h.clock.Advance(auth.VerifyTTL + time.Minute)
	if body := page(); !strings.Contains(body, "The link to <strong>ada@example.com</strong> does not work any more.") {
		t.Error("the page does not say that the link expired")
	}
	w = h.do(req{method: "POST", path: "/api/auth/verify-email/resend", body: map[string]any{}, as: ada})
	wantStatus(t, w, http.StatusOK)
	if got := decodeJSON[map[string]string](t, w)["email"]; got != "ada@example.com" {
		t.Errorf("resend went to %q", got)
	}
	if body := page(); !strings.Contains(body, "We sent a link to <strong>ada@example.com</strong>.") {
		t.Error("the page does not show the new link")
	}
	// The limits of docs/DESIGN.md answer with their own message, which
	// the page shows in its status line.
	var limited *httptest.ResponseRecorder
	for range 6 {
		w := h.do(req{method: "POST", path: "/api/auth/verify-email/resend", body: map[string]any{}, as: ada})
		if w.Code == http.StatusTooManyRequests {
			limited = w
			break
		}
		wantStatus(t, w, http.StatusOK)
	}
	if limited == nil {
		t.Fatal("resending is not rate limited")
	}
	if body := decodeJSON[map[string]string](t, limited); body["code"] != "rate_limited" || body["error"] == "" || limited.Header().Get("Retry-After") == "" {
		t.Errorf("rate limit answer = %v, Retry-After %q", body, limited.Header().Get("Retry-After"))
	}

	// A change of address on the account page shows on this page.
	h.clock.Advance(time.Hour)
	w = h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Ada Admin", "email": "ada@example.org"}, as: ada})
	wantStatus(t, w, http.StatusOK)
	if body := page(); !strings.Contains(body, "We sent a link to <strong>ada@example.org</strong>.") {
		t.Error("the page does not show the new address")
	}

	// A verified user has nothing to wait for.
	ada.Email = "ada@example.org"
	ada = h.verify(ada)
	w = h.do(req{method: "GET", path: "/check-email", as: ada})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/learn" {
		t.Errorf("verified /check-email = %d to %q, want 303 to /learn", w.Code, w.Header().Get("Location"))
	}
}

// TestCheckEmailWithoutAddress: an account from before the address was
// required has none. It stays behind the gate and is told to add one.
func TestCheckEmailWithoutAddress(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	old := h.unverifiedLearner(admin, "olaf")
	if err := h.store.SetEmail(t.Context(), old.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Such an account never got a link; the one of this sign-up expires.
	h.clock.Advance(auth.VerifyTTL + time.Minute)
	if w := h.do(req{method: "GET", path: "/learn", as: old}); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/check-email" {
		t.Fatalf("/learn without an address = %d to %q", w.Code, w.Header().Get("Location"))
	}
	w := h.do(req{method: "GET", path: "/check-email", as: old})
	wantStatus(t, w, http.StatusOK)
	body := w.Body.String()
	for _, want := range []string{"Your account has no email address. Add an address to start the course.", `href="/account">Add an address</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, "Send the link again") {
		t.Error("the page offers to send a link to no address")
	}
	w = h.do(req{method: "POST", path: "/api/auth/verify-email/resend", body: map[string]any{}, as: old})
	wantStatus(t, w, http.StatusBadRequest)
	// The account page can add one, and cannot leave it empty.
	w = h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Olaf", "email": ""}, as: old})
	wantStatus(t, w, http.StatusBadRequest)
	w = h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Olaf", "email": "olaf@example.com"}, as: old})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(h.do(req{method: "GET", path: "/check-email", as: old}).Body.String(), "We sent a link to <strong>olaf@example.com</strong>.") {
		t.Error("the page does not show the added address")
	}
}

func TestEmailRequired(t *testing.T) {
	h := newHarness(t)
	link, err := h.auth.EnsureSetupLink(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	setup, _ := url.Parse(link)
	wantRequired := func(w *httptest.ResponseRecorder) {
		t.Helper()
		wantStatus(t, w, http.StatusBadRequest)
		body := decodeJSON[map[string]string](t, w)
		if body["code"] != "email_required" || body["error"] != "Enter your email address. It is your way back in if you lose your passkey." {
			t.Errorf("answer = %v, want email_required", body)
		}
	}
	// Setup.
	wantRequired(h.do(req{method: "POST", path: "/api/auth/register/begin", body: map[string]string{
		"setup": setup.Query().Get("token"), "username": "ada", "displayName": "Ada",
	}}))
	admin := h.admin()

	// Join.
	res, err := h.auth.CreateInvite(t.Context(), admin.User, auth.InviteRequest{}, auth.Client{})
	if err != nil {
		t.Fatal(err)
	}
	invite, _ := url.Parse(res.Link)
	wantRequired(h.do(req{method: "POST", path: "/api/auth/register/begin", body: map[string]string{
		"invite": invite.Query().Get("invite"), "username": "bob", "displayName": "Bob", "email": "  ",
	}}))
	page := h.do(req{method: "GET", path: "/join?invite=" + invite.Query().Get("invite")}).Body.String()
	if !strings.Contains(page, `id="join-email" name="email" type="email" required`) {
		t.Error("the join form does not mark the address required")
	}

	// The account page cannot clear the address, verified or not.
	bob := h.unverifiedLearner(admin, "bob")
	for _, u := range []*user{admin, bob} {
		wantRequired(h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": u.DisplayName, "email": ""}, as: u}))
		if got, err := h.store.User(t.Context(), u.ID); err != nil || got.Email != u.Email {
			t.Errorf("%s after a refused clear: %q, %v", u.Username, got.Email, err)
		}
	}
	if page := h.do(req{method: "GET", path: "/account", as: bob}).Body.String(); !strings.Contains(page, "Not verified. Open the link in the email to start the course.") {
		t.Error("the account page does not say that the address is not verified")
	}
	if page := h.do(req{method: "GET", path: "/account", as: admin}).Body.String(); !strings.Contains(page, "verified</span> Recovery links") {
		t.Error("the account page does not say that the address is verified")
	}
}

// TestSignUpNext: a sign-up goes to /learn only when a mailed invite proved
// the address; every other one waits on /check-email.
func TestSignUpNext(t *testing.T) {
	h := newHarness(t)
	link, err := h.auth.EnsureSetupLink(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	setup, _ := url.Parse(link)
	ada, next := h.signUpNext(map[string]string{"setup": setup.Query().Get("token"), "username": "ada", "displayName": "Ada", "email": "ada@example.com"})
	if next != "/check-email" || ada.EmailVerified() {
		t.Fatalf("setup: next %q, verified %v", next, ada.EmailVerified())
	}
	ada = h.verify(ada)

	// invite creates an invite as the admin page does and returns its
	// token from wherever the link went.
	invite := func(email string, smtp bool) string {
		t.Helper()
		w := h.do(req{method: "POST", path: "/api/admin/invites", body: map[string]any{"email": email}, as: ada})
		wantStatus(t, w, http.StatusCreated)
		res := decodeJSON[map[string]any](t, w)
		link, _ := res["url"].(string)
		// A mail that an SMTP server took holds the only copy of the link,
		// so a copy the admin passes on cannot pose as proof.
		if mailed := smtp && email != ""; (link == "") != mailed || res["mailed"] != (email != "") {
			t.Fatalf("invite answer = %v; want a url: %v", res, !mailed)
		}
		if link == "" {
			return h.lastInvite(email)
		}
		u, _ := url.Parse(link)
		return u.Query().Get("invite")
	}
	tests := []struct {
		name     string
		smtp     bool
		invite   string
		signUp   string
		wantNext string
	}{
		{"mailed, same address", true, "Bob@Example.com", "bob@example.com", "/learn"},
		{"mailed, other address", true, "cat@example.com", "cat@example.org", "/check-email"},
		{"logged, not mailed", false, "dan@example.com", "dan@example.com", "/check-email"},
		{"invite without an address", true, "", "eve@example.com", "/check-email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.mails.setDelivers(tt.smtp)
			token := invite(tt.invite, tt.smtp)
			page := h.do(req{method: "GET", path: "/join?invite=" + token}).Body.String()
			if mailed := strings.Contains(page, "The invite came to this address, so it counts as verified."); mailed != (tt.smtp && tt.invite != "") {
				t.Errorf("the join page says mailed: %v", mailed)
			}
			name := strings.SplitN(tt.signUp, "@", 2)[0]
			u, next := h.signUpNext(map[string]string{"invite": token, "username": name, "displayName": name, "email": tt.signUp})
			if next != tt.wantNext {
				t.Errorf("next = %q, want %q", next, tt.wantNext)
			}
			wantPage := http.StatusOK
			if tt.wantNext != "/learn" {
				wantPage = http.StatusSeeOther
			}
			wantStatus(t, h.do(req{method: "GET", path: "/learn", as: u}), wantPage)
		})
	}
}

// TestResendAfterVerify: the check-email page in a tab that stayed open
// while the link was used on another device. "Send the link again" learns
// that nothing waits, and no mail goes out; the page then goes to /learn.
func TestResendAfterVerify(t *testing.T) {
	h := newHarness(t)
	ada := h.unverifiedAdmin()
	ada = h.verify(ada)
	h.mails.mu.Lock()
	sent := len(h.mails.msgs)
	h.mails.mu.Unlock()
	w := h.do(req{method: "POST", path: "/api/auth/verify-email/resend", body: map[string]any{}, as: ada})
	wantStatus(t, w, http.StatusConflict)
	if body := decodeJSON[map[string]string](t, w); body["code"] != "already_verified" || body["error"] == "" {
		t.Errorf("answer = %v, want already_verified", body)
	}
	h.auth.Wait()
	h.mails.mu.Lock()
	defer h.mails.mu.Unlock()
	if len(h.mails.msgs) != sent {
		t.Errorf("%d mails after the refused resend, want %d", len(h.mails.msgs), sent)
	}
}
