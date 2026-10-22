package web_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The learner's pages in the Vesper Labs design (VESPER.md 8.6 to 8.11),
// rendered with the site's own templates for a learner in tier 2 with
// the tag of tier 1 stamped.
func TestLearnerPages(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	ada := h.learner(admin, "noor")
	ctx := t.Context()
	for _, e := range testCourse.Tiers[0].Exercises {
		if _, err := h.store.MarkPassed(ctx, ada.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}
	w := h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "tier1", "showStats": true}, as: ada})
	wantStatus(t, w, http.StatusCreated)
	issued := decodeJSON[map[string]string](t, w)
	rollcall := testCourse.Tiers[1].Exercises[1]
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/" + rollcall.Slug, as: ada}), http.StatusOK)

	page := func(path string, as *user) (raw, text string) {
		t.Helper()
		w := h.do(req{method: "GET", path: path, as: as})
		wantStatus(t, w, http.StatusOK)
		raw = w.Body.String()
		// No page carries an inline style, script or handler: the CSP
		// refuses all 3 outside the IDE, which allows inline styles for
		// Monaco, so this check covers the IDE's markup too.
		for _, bad := range []string{` style=`, ` onclick=`, `<script>`} {
			if strings.Contains(raw, bad) {
				t.Errorf("GET %s has %q", path, bad)
			}
		}
		return raw, visibleText(raw)
	}
	want := func(path, what, page string, wants ...string) {
		t.Helper()
		for _, s := range wants {
			if !strings.Contains(page, s) {
				t.Errorf("GET %s: the %s lacks %q", path, what, s)
			}
		}
	}

	raw, text := page("/learn", ada)
	want("/learn", "page", text, "Your page of the Ledger", "5 of 50 tools are in the Ledger.", "You are on call for",
		"Continue exercise 07", "Breaker 0, Hall 3", "1 of 10 tags", "The Key is on your ring: you share the rotation with Wren.",
		"stamped", "5 to go", "Tier 2, The Key", "1 to 3 are hints.", "Tier 1, Badge In", "5 of 5 in the Ledger", "Tag stamped",
		"Tiers 3 to 10", "waiting for a pass", "Calm Score",
		// The slab's heading names the region: a space after the number.
		"07 "+rollcall.Title)
	want("/learn", "markup", raw, `class="gpw-oncall gpw-oncall-key"`, `class="gpw-ring-draw"`, `class="gpw-sky gpw-sky-dash"`,
		`<tr class="gpw-cur">`, `id="tier-2-log">`, `id="tier-1-log" hidden`, `aria-controls="tier-1-log" data-fold>Show`,
		`class="gpw-telem"`)
	// The count is said once: the big numeral and its hidden sentence.
	if n := strings.Count(text, "5 of 50"); n != 1 {
		t.Errorf("the dashboard says the count %d times", n)
	}

	raw, text = page("/certificates", ada)
	want("/certificates", "page", text, "1 of 11 certificates is yours and current.", "Tags stamped", "1 of 10",
		"completed Tier 1, Badge In: you read the raw lines the dashboards miss.", "Held", "In progress", "Not started",
		"Open the PDF", "Issue again", "The Key: the course certificate", "5 of 50 in the Ledger.")
	want("/certificates", "markup", raw, `class="gpw-ct-row gpw-ct-held"`, `class="gpw-ct-plate"`, `data-kind="tier1"`,
		`href="`+issued["pdf"]+`"`, `class="gpw-ct-keysvg"`)

	raw, text = page(issued["url"], nil)
	want(issued["url"], "page", text, "Valid.", "The go-practice course issued this certificate to", "Tier certificate",
		"completed Tier 1, Badge In, of the go-practice course: you read the raw lines the dashboards miss.",
		"Each of the 5 exercises passed its full test suite with go test -race.", "Scan to verify", issued["id"],
		"The owner chose to show it.", "Show each exercise", "Download the PDF")
	want(issued["url"], "markup", raw, `class="gpw-lighttable"`, `class="gpw-paper"`, `class="gpw-pp-qrsvg"`)
	if regexp.MustCompile(`gpw-sheet[" ]`).MatchString(raw) {
		t.Error("the light table uses gpw-sheet, the menu sheet's frame name")
	}

	_, text = page("/account", ada)
	want("/account", "page", text, "Signed in as noor", "Verified.", "Show story effects",
		"AI assistance is off in this IDE. Rule 1.", "14 px, the default", "Sign out everywhere")

	raw, text = page("/admin", admin)
	want("/admin", "page", text, "Create an invite", "Invites", "Users", "5 of 50", "You cannot disable your own account.")
	want("/admin", "markup", raw, `class="gpw-sky gpw-sky-mini"`)
	if regexp.MustCompile(`btn-danger`).MatchString(raw) {
		t.Error("a row action on the admin page is red: only a WARNING dialog's button is")
	}
}
