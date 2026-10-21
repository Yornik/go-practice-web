package web_test

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
)

func TestPagesRender(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	tests := []struct {
		path string
		as   *user
		want []string
	}{
		{"/", nil, []string{"Practice Go on real operations tasks", "Tier 1", "Sign in", "Figure 2", "is not declared where you use it"}},
		{"/login", nil, []string{"Sign in with a passkey", `autocomplete="username webauthn"`}},
		{"/guide", nil, []string{"The guide", "The rules", "How to read a Go error message", "Print the checklist"}},
		{"/recover", nil, []string{"Recover your account"}},
		{"/verify", nil, []string{"Verify a certificate"}},
		{"/verify-email?token=x", nil, []string{"Confirm my address"}},
		{"/learn", admin, []string{"Your course", "learning log", "Parse logfmt log lines", `data-stat="passed"`}},
		{"/learn/" + logfmt, admin, []string{"data-exercise=\"" + logfmt + "\"", "Run tests", "Hint 1", "All tests", "A nudge"}},
		{"/account", admin, []string{"Passkeys", "ada@example.com", "Sign out everywhere"}},
		{"/certificates", admin, []string{"Tier 1 · Badge In", "The complete course", "5 to go: 01, 02, 03, 04, 05."}},
		{"/admin", admin, []string{"Create an invite", "Ada Admin"}},
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
	for p, want := range map[string]string{
		"/join?invite=nope":           "This invite link does not work. Check that you copied all of it.",
		"/setup?token=nope":           "This setup link does not work any more.",
		"/recover/confirm?token=nope": "This recovery link does not work any more. It works once, for 15 minutes.",
	} {
		w := h.do(req{method: "GET", path: p})
		wantStatus(t, w, http.StatusGone)
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s lacks the dead-link message %q", p, want)
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

	// An expired invite says when it expired.
	inviter := admin
	res, err := h.auth.CreateInvite(t.Context(), inviter.User, auth.InviteRequest{Days: 1}, auth.Client{})
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(48 * time.Hour)
	w = h.do(req{method: "GET", path: strings.TrimPrefix(res.Link, publicURL)})
	wantStatus(t, w, http.StatusGone)
	if body := w.Body.String(); !strings.Contains(body, "This invite link expired on 18") || !strings.Contains(body, "Oct 2026.") {
		t.Error("the expired invite page lacks its date")
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
	// The row the page adds names the creator as the table does.
	if res["createdBy"] != "ada" {
		t.Errorf("createdBy = %v, want the username", res["createdBy"])
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

	// An invite that an SMTP server took counts as proof of its address,
	// so the mail holds the only copy of the link: the answer has no url.
	h.mails.setDelivers(true)
	w = h.do(req{method: "POST", path: "/api/admin/invites", body: map[string]any{"email": "cat@example.com"}, as: admin})
	wantStatus(t, w, http.StatusCreated)
	res = decodeJSON[map[string]any](t, w)
	if _, ok := res["url"]; ok || res["mailed"] != true || res["hash"] == "" {
		t.Fatalf("SMTP-mailed invite = %v, want no url", res)
	}
	if token := h.lastInvite("cat@example.com"); token == "" {
		t.Fatal("the invite mail has no link")
	}
}
