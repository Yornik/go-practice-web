package web_test

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func TestPagesRender(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	tests := []struct {
		path string
		as   *user
		want []string
	}{
		{"/", nil, []string{"Learn Go", "Tier 1", "Sign in"}},
		{"/login", nil, []string{"Sign in with a passkey", `autocomplete="username webauthn"`}},
		{"/guide", nil, []string{"How the course works", "The rules"}},
		{"/recover", nil, []string{"Recover your account"}},
		{"/verify", nil, []string{"Verify a certificate"}},
		{"/verify-email?token=x", nil, []string{"Confirm my address"}},
		{"/learn", admin, []string{"Your progress", "Learning log", "01 · "}},
		{"/learn/01-logfmt", admin, []string{"data-exercise=\"01-logfmt\"", "Run the tests", "Hint 1", "All tests"}},
		{"/account", admin, []string{"Passkeys", "ada@example.com", "Sign out everywhere"}},
		{"/certificates", admin, []string{"Tier 1", "The complete course"}},
		{"/admin", admin, []string{"Invite someone", "Ada Admin"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := h.do(req{method: "GET", path: tt.path, as: tt.as})
			wantStatus(t, w, http.StatusOK)
			if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Fatalf("CSP = %q", csp)
			}
			body := w.Body.String()
			for _, s := range tt.want {
				if !strings.Contains(body, s) {
					t.Errorf("page lacks %q", s)
				}
			}
		})
	}
}

func TestLinkPages(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/join?invite=nope", "/setup?token=nope", "/recover/confirm?token=nope"} {
		w := h.do(req{method: "GET", path: p})
		wantStatus(t, w, http.StatusGone)
		if !strings.Contains(w.Body.String(), "already used or has expired") {
			t.Errorf("%s lacks the dead-link message", p)
		}
	}
	link, err := h.auth.EnsureSetupLink(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	w := h.do(req{method: "GET", path: strings.TrimPrefix(link, publicURL)})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `data-mode="setup"`) {
		t.Fatal("setup page lacks the form")
	}
	// A signed-in visitor of /login goes on.
	admin := h.admin()
	w = h.do(req{method: "GET", path: "/login?next=/account", as: admin})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/account" {
		t.Fatalf("login when signed in = %d %s", w.Code, w.Header().Get("Location"))
	}
	w = h.do(req{method: "GET", path: "/login?next=//evil.example", as: admin})
	if w.Header().Get("Location") != "/learn" {
		t.Fatalf("open redirect: %s", w.Header().Get("Location"))
	}
}

func TestNotFoundAndHealth(t *testing.T) {
	h := newHarness(t)
	wantStatus(t, h.do(req{method: "GET", path: "/nope"}), http.StatusNotFound)
	w := h.do(req{method: "GET", path: "/api/nope"})
	wantStatus(t, w, http.StatusNotFound)
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("API 404 is %s", w.Header().Get("Content-Type"))
	}
	wantStatus(t, h.do(req{method: "GET", path: "/healthz"}), http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/readyz"}), http.StatusOK)
	wantStatus(t, h.do(req{method: "GET", path: "/learn/99-nope", as: h.admin()}), http.StatusNotFound)
}

func TestStaticFiles(t *testing.T) {
	h := newHarness(t)
	w := h.do(req{method: "GET", path: "/static/css/site.css"})
	wantStatus(t, w, http.StatusOK)
	if w.Header().Get("Content-Type") != "text/css; charset=utf-8" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v", w.Header())
	}
	w = h.do(req{method: "GET", path: "/static/js/api.js"})
	wantStatus(t, w, http.StatusOK)
	if w.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("js type = %s", w.Header().Get("Content-Type"))
	}
	for _, p := range []string{"/static/", "/static/js/", "/static/../go.mod", "/static/nope.js"} {
		// The mux answers a path with ".." with a redirect to the clean
		// path, which is outside /static/.
		if w := h.do(req{method: "GET", path: p}); w.Code != http.StatusNotFound && w.Code != http.StatusTemporaryRedirect {
			t.Errorf("GET %s = %d", p, w.Code)
		}
	}
	w = h.do(req{method: "GET", path: "/highlight.css"})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), "[data-theme") {
		t.Fatal("highlight CSS lacks the theme scope")
	}
}

func TestAdminInvites(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	w := h.do(req{method: "POST", path: "/api/admin/invites", body: map[string]any{"email": "bob@example.com", "note": "Welcome", "days": 3}, as: admin})
	wantStatus(t, w, http.StatusCreated)
	res := decodeJSON[map[string]any](t, w)
	if res["mailed"] != true || !strings.HasPrefix(res["url"].(string), publicURL+"/join?invite=") {
		t.Fatalf("invite = %v", res)
	}
	invites, err := h.store.Invites(t.Context())
	if err != nil || len(invites) != 1 {
		t.Fatalf("invites = %v, %v", invites, err)
	}
	page := h.do(req{method: "GET", path: "/admin", as: admin}).Body.String()
	if !strings.Contains(page, "bob@example.com") || !strings.Contains(page, "Open until") {
		t.Fatal("admin page lacks the invite")
	}
	hash := hex.EncodeToString(invites[0].TokenHash)
	wantStatus(t, h.do(req{method: "DELETE", path: "/api/admin/invites/" + hash, as: admin}), http.StatusOK)
	wantStatus(t, h.do(req{method: "DELETE", path: "/api/admin/invites/" + hash, as: admin}), http.StatusNotFound)
	w = h.do(req{method: "POST", path: "/api/admin/invites", body: map[string]any{"days": 99}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)
}
