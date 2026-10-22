package web_test

import (
	"html/template"
	"net/http"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/web"
)

// The disclaimer is the landing page's own last element (VESPER.md 8.1):
// no other page carries it.
func TestDisclaimerOnlyOnTheLanding(t *testing.T) {
	h := newHarness(t)
	const disclaimer = "Vesper Labs, HUSH, Meridian and everybody in this story are fictional."
	body := h.do(req{method: "GET", path: "/"}).Body.String()
	i := strings.Index(body, disclaimer)
	if i < 0 {
		t.Fatal("the landing page has no disclaimer")
	}
	// No text follows it: the toast region after the footer is empty.
	rest := body[i:]
	rest = rest[strings.Index(rest, "</p>"):]
	if text := strings.TrimSpace(pageText(rest)); text != "" {
		t.Errorf("text follows the disclaimer: %q", text)
	}
	for _, p := range []string{"/guide", "/login", "/recover", "/verify", "/no-such-page"} {
		if strings.Contains(h.do(req{method: "GET", path: p}).Body.String(), disclaimer) {
			t.Errorf("%s carries the disclaimer", p)
		}
	}
}

// GPW_MOVED_FROM shows the note about passkeys of the old host on the
// sign-in and recovery pages, and nothing when it is empty.
func TestMovedFromNote(t *testing.T) {
	h := newHarness(t)
	for _, p := range []string{"/login", "/recover"} {
		if strings.Contains(h.do(req{method: "GET", path: p}).Body.String(), "Did you use") {
			t.Errorf("%s shows the move note, but the site did not move", p)
		}
	}
	cfg := h.cfg
	cfg.MovedFrom = "go-learn.party"
	srv, err := web.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.handler = srv.Handler()
	host := strings.TrimPrefix(strings.TrimPrefix(publicURL, "https://"), "http://")
	for p, button := range map[string]bool{"/login": true, "/recover": false} {
		text := pageText(h.do(req{method: "GET", path: p}).Body.String())
		for _, want := range []string{
			"Did you use go-learn.party?",
			"Go Practice moved from go-learn.party to " + host + ". Your account and your progress moved with it.",
			"So a passkey made on go-learn.party does not work here.",
			"Recover your account to add a new passkey.",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", p, want)
			}
		}
		// The recovery page is where the button would lead.
		if got := strings.Contains(h.do(req{method: "GET", path: p}).Body.String(), `<a class="btn" href="/recover">Recover your account</a>`); got != button {
			t.Errorf("%s has the recovery button: %v, want %v", p, got, button)
		}
	}
}

// The 404 lists the asked-for path in its roll call: escaped, cut to 60
// runes, never a link. 403 keeps the head and drops the roll call.
func TestErrorPages(t *testing.T) {
	h := newHarness(t)
	long := "/learn/" + strings.Repeat("x", 40) + "<script>alert(1)</script>" + strings.Repeat("y", 30)
	w := h.do(req{method: "GET", path: long})
	wantStatus(t, w, http.StatusNotFound)
	body := w.Body.String()
	for _, want := range []string{
		"This page is not in the inventory.", "If it starts sending heartbeats, tell Noor.",
		`<tr class="gpw-po-miss"><td data-label="State">unknown</td>`, template.HTMLEscapeString(long[:59]) + "…",
		"5 checked: 4 healthy, 1 unknown. Exit status 1.", "there is no floor 404.",
		`F4<tspan class="gpw-sv-z">0</tspan>4`, `<span class="gpw-zs">4<span class="gpw-z">0</span>4</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the 404 page lacks %q", want)
		}
	}
	// Page titles are in sentence case (VESPER.md 10.2), not StatusText's.
	if !strings.Contains(body, "<title>Page not found, Vesper Labs Go Practice</title>") {
		t.Error("the 404 page's title is not \"Page not found, Vesper Labs Go Practice\"")
	}
	if strings.Contains(body, "<script>alert") || strings.Contains(body, `href="/learn/xxx`) {
		t.Error("the asked-for path is not plain, escaped text")
	}

	admin := h.admin()
	noor := h.learner(admin, "noor")
	w = h.do(req{method: "GET", path: "/admin", as: noor})
	wantStatus(t, w, http.StatusForbidden)
	body = w.Body.String()
	for _, want := range []string{"<title>No access, Vesper Labs Go Practice</title>", "You do not have access to this page.", "Admin pages need an admin account.", `<span class="gpw-zs">4<span class="gpw-z">0</span>3</span>`, `href="/learn">Your course</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("the 403 page lacks %q", want)
		}
	}
	if strings.Contains(body, "gpw-po") || strings.Contains(body, "not in the inventory") {
		t.Error("the 403 page tells the 404's story")
	}
}

// A store that fails while the session loads must not hand a browser raw
// JSON: a page answers with the 5xx page, an API with its JSON error.
func TestDatabaseDownWithASession(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	w := h.do(req{method: "GET", path: "/verify/GP-2345-6789-ABCD", as: admin})
	wantStatus(t, w, http.StatusServiceUnavailable)
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	text := pageText(w.Body.String())
	for _, want := range []string{"The server could not finish this request.", "The site cannot reach its database. Try again in a minute."} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	w = h.do(req{method: "GET", path: "/api/certificates", as: admin})
	wantStatus(t, w, http.StatusServiceUnavailable)
	if got := decodeJSON[map[string]string](t, w)["code"]; got != "unavailable" {
		t.Errorf("code = %q, want unavailable", got)
	}
}

// The check-email page names the sender and the site the link starts
// with, so the learner knows what to look for.
func TestCheckEmailNamesTheSender(t *testing.T) {
	h := newHarness(t)
	ada := h.unverifiedAdmin()
	text := pageText(h.do(req{method: "GET", path: "/check-email", as: ada}).Body.String())
	for _, want := range []string{
		"The mail comes from HUSH · Vesper Labs, hush@vesper-labs.org.",
		"The link in it starts with " + publicURL + "/.",
		"Sent to", "ada@example.com", "Works for",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}
