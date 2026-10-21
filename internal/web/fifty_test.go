package web_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/cert"
	"github.com/yornik/go-practice-web/internal/course"
	"github.com/yornik/go-practice-web/internal/store"
)

// These tests are about the course of 50, in which the first course's 25
// exercises are tiers 6 to 10 under new slugs.

// TestRenamedSlugRedirects pins where an old /learn/ link goes, and that
// the email gate comes first: a renamed slug answers only to a learner who
// may open the course at all.
func TestRenamedSlugRedirects(t *testing.T) {
	h := newHarness(t)
	admin := h.unverifiedAdmin()

	// Without a session: the sign-in page, with the old path to come back to.
	w := h.do(req{method: "GET", path: "/learn/07-retry"})
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/login?next=") ||
		!strings.Contains(w.Header().Get("Location"), "07-retry") {
		t.Fatalf("anonymous = %d %q", w.Code, w.Header().Get("Location"))
	}
	// An unverified address: the gate answers before the redirect.
	w = h.do(req{method: "GET", path: "/learn/07-retry", as: admin})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/check-email" {
		t.Fatalf("unverified = %d %q", w.Code, w.Header().Get("Location"))
	}
	admin = h.verify(admin)
	for old, current := range map[string]string{"07-retry": "32-retry", "01-logfmt": "26-logfmt", "25-loadbalancer": "50-loadbalancer"} {
		w = h.do(req{method: "GET", path: "/learn/" + old, as: admin})
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/learn/"+current {
			t.Errorf("GET /learn/%s = %d %q, want 302 to /learn/%s", old, w.Code, w.Header().Get("Location"), current)
		}
	}
	w = h.do(req{method: "GET", path: "/learn/32-retry", as: admin})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `data-exercise="32-retry"`) {
		t.Error("the IDE of 32-retry does not name its slug")
	}
	// The browser may keep unsaved code under the old slug: the page names
	// it, so the IDE moves that code to the new slug.
	if !strings.Contains(w.Body.String(), `data-renamed-from="07-retry"`) {
		t.Error("the IDE of 32-retry does not name its old slug")
	}
	// A new exercise that reuses an old number is itself, not a redirect.
	w = h.do(req{method: "GET", path: "/learn/07-rollcall", as: admin})
	wantStatus(t, w, http.StatusOK)
	if strings.Contains(w.Body.String(), "data-renamed-from") {
		t.Error("the IDE of 07-rollcall names an old slug")
	}
	wantStatus(t, h.do(req{method: "GET", path: "/learn/99-nothing", as: admin}), http.StatusNotFound)

	// The API takes current slugs only.
	for _, r := range []req{
		{method: "GET", path: "/api/exercises/07-retry"},
		{method: "POST", path: "/api/exercises/07-retry/check", body: map[string]any{"files": map[string]string{}}},
		{method: "PUT", path: "/api/exercises/07-retry/files/retry.go", body: map[string]string{"content": "package retry"}},
		{method: "POST", path: "/api/exercises/07-retry/reveal", body: map[string]any{"level": 1}},
	} {
		r.as = admin
		w := h.do(r)
		// An IDE tab that was open while the course grew learns where the
		// exercise went, so it stops trying to save under the old slug.
		var body struct{ Code, Slug, Error string }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != http.StatusNotFound || body.Code != "renamed" || body.Slug != "32-retry" || !strings.Contains(body.Error, "32-retry") {
			t.Errorf("%s %s = %d %s, want 404 renamed to 32-retry", r.method, r.path, w.Code, w.Body)
		}
	}
	w = h.do(req{method: "GET", path: "/api/exercises/99-nothing", as: admin})
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"unknown_exercise"`) {
		t.Errorf("GET /api/exercises/99-nothing = %d %s", w.Code, w.Body)
	}
	wantStatus(t, h.do(req{method: "GET", path: "/api/exercises/32-retry", as: admin}), http.StatusOK)
}

// issueOld stores a certificate the first course signed, as migration
// 0003 leaves it: the payload names the old kind and slugs, the row the
// tier that holds those exercises now.
func issueOld(t *testing.T, h *harness, u *user, kind cert.Kind, storedKind string, slugs []string) store.Certificate {
	t.Helper()
	c, err := h.signer.Issue(cert.Request{Kind: kind, Name: u.DisplayName, Exercises: slugs, IssuedAt: h.clock.Now()})
	if err != nil {
		t.Fatal(err)
	}
	issued, _ := c.Payload.Issued()
	row := store.Certificate{ID: c.Payload.ID, UserID: u.ID, Kind: storedKind, NameOnCert: c.Payload.Name, IssuedAt: issued,
		Payload: c.Canonical, Signature: c.Signature}
	if _, err := h.store.IssueCertificate(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	return row
}

// firstCourseSlugs returns the slugs of the first course, from the rename
// table: the old slugs of every renamed exercise, in number order.
func firstCourseSlugs(c *course.Course) []string {
	var out []string
	for _, e := range c.Exercises {
		for old, current := range course.Renames() {
			if current == e.Slug {
				out = append(out, old)
			}
		}
	}
	return out
}

func TestEarlierCertificates(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	old := firstCourseSlugs(testCourse)
	if len(old) != 25 {
		t.Fatalf("the course renamed %d exercises", len(old))
	}
	tier1 := issueOld(t, h, admin, "tier1", "tier6", old[:3])
	courseCert := issueOld(t, h, admin, cert.KindCourse, "course", old)
	// The migration moved the first course's passes to the new slugs.
	for _, e := range testCourse.Exercises[25:] {
		if _, err := h.store.MarkPassed(t.Context(), admin.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}

	type kindJSON struct {
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Eligible    bool   `json:"eligible"`
		Total       int    `json:"total"`
		Certificate *struct {
			ID string `json:"id"`
		} `json:"certificate"`
		Earlier *struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"earlier"`
	}
	status := func() map[string]kindJSON {
		w := h.do(req{method: "GET", path: "/api/certificates", as: admin})
		wantStatus(t, w, http.StatusOK)
		out := map[string]kindJSON{}
		for _, k := range decodeJSON[struct {
			Kinds []kindJSON `json:"kinds"`
		}](t, w).Kinds {
			out[k.Kind] = k
		}
		return out
	}
	st := status()
	if len(st) != 11 {
		t.Fatalf("%d kinds, want 11", len(st))
	}
	// The old tier1 covers exactly tier 6 of the course of 50: held.
	if k := st["tier6"]; k.Certificate == nil || k.Certificate.ID != tier1.ID || k.Earlier != nil || k.Title != "Tier 6 · Forensics" {
		t.Errorf("tier6 = %+v", k)
	}
	// The old course certificate covers 25 of 50: an earlier one.
	if k := st["course"]; k.Certificate != nil || k.Earlier == nil || k.Earlier.ID != courseCert.ID || k.Eligible || k.Total != 50 {
		t.Errorf("course = %+v", k)
	}

	page := h.do(req{method: "GET", path: "/certificates", as: admin}).Body.String()
	for _, want := range []string{"Tier 6 · Forensics", "The complete course", "50 exercises in 10 tiers",
		"an earlier certificate", courseCert.ID, "Tier 2 · The Key"} {
		if !strings.Contains(page, want) {
			t.Errorf("the certificates page lacks %q", want)
		}
	}
	if strings.Contains(page, `data-claim="again">Claim`) {
		t.Error("the course can be claimed before every exercise passes")
	}

	// The verify page reads the old payloads through the renames.
	v := h.do(req{method: "GET", path: "/verify/" + courseCert.ID}).Body.String()
	// It also says what the certificate was issued as: its PDF of then
	// shows the first course's numbers.
	for _, want := range []string{"Valid.", "The complete course", "25 exercises in 5 tiers", "Forensics", "26 to 50", "--tier-max: 25",
		"Issued for the first course: 25 exercises in 5 tiers, numbered 01 to 25. The current course numbers them 26 to 50, in tiers 6 to 10."} {
		if !strings.Contains(v, want) {
			t.Errorf("the verify page of the old course certificate lacks %q", want)
		}
	}
	if n := len(regexp.MustCompile(`data-state="passed"`).FindAllString(v, -1)); n != 25 {
		t.Errorf("the strip of the old course certificate has %d bars, want 25", n)
	}
	v = h.do(req{method: "GET", path: "/verify/" + tier1.ID}).Body.String()
	for _, want := range []string{"Tier 6 · Forensics", "Warm-up: Go idioms on real data", "Parse logfmt log lines", "26 to 28",
		"Issued as Tier 1 of the first course, exercises 01 to 03. The current course numbers them 26 to 28, in Tier 6 · Forensics."} {
		if !strings.Contains(v, want) {
			t.Errorf("the verify page of the old tier1 lacks %q", want)
		}
	}
	w := h.do(req{method: "GET", path: "/verify/" + courseCert.ID + ".pdf"})
	wantStatus(t, w, http.StatusOK)

	// With every exercise passed, the course can be claimed again; the
	// claim replaces the earlier certificate.
	for _, e := range testCourse.Exercises[:25] {
		if _, err := h.store.MarkPassed(t.Context(), admin.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}
	if k := status()["course"]; !k.Eligible || k.Earlier == nil {
		t.Fatalf("course after all passes = %+v", k)
	}
	page = h.do(req{method: "GET", path: "/certificates", as: admin}).Body.String()
	if !strings.Contains(page, `data-claim="again">Claim the course certificate`) {
		t.Error("the certificates page offers no claim over the earlier certificate")
	}
	w = h.do(req{method: "POST", path: "/api/certificates", body: map[string]any{"kind": "course", "name": "Ada Admin"}, as: admin})
	wantStatus(t, w, http.StatusCreated)
	claimed := decodeJSON[map[string]string](t, w)
	if claimed["replaced"] != courseCert.ID {
		t.Errorf("the claim replaced %q, want %q", claimed["replaced"], courseCert.ID)
	}
	if k := status()["course"]; k.Certificate == nil || k.Certificate.ID != claimed["id"] || k.Earlier != nil {
		t.Errorf("course after the claim = %+v", k)
	}
	v = h.do(req{method: "GET", path: "/verify/" + courseCert.ID}).Body.String()
	if !strings.Contains(v, "Revoked.") || !strings.Contains(v, claimed["id"]) {
		t.Error("the earlier certificate is not revoked in favour of the new one")
	}
	v = h.do(req{method: "GET", path: claimed["url"]}).Body.String()
	for _, want := range []string{"50 exercises in 10 tiers", "The Key Turns", "01 to 50"} {
		if !strings.Contains(v, want) {
			t.Errorf("the verify page of the new course certificate lacks %q", want)
		}
	}
	if strings.Contains(v, "data-issued-as") {
		t.Error("the verify page of a new certificate says what it was issued as")
	}
	// The tier certificate that was current stays as it was.
	if k := status()["tier6"]; k.Certificate == nil || k.Certificate.ID != tier1.ID {
		t.Errorf("tier6 after the course claim = %+v", k)
	}
}

func TestFiftyPages(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	if _, err := h.store.StartExercise(t.Context(), admin.ID, "32-retry"); err != nil {
		t.Fatal(err)
	}

	landing := h.do(req{method: "GET", path: "/"}).Body.String()
	for _, want := range []string{
		"50 exercises in 10 tiers", "The blue bar is exercise 32.", "scenario of exercise 32", "A compile error in exercise 32",
		"<dt>Exercise</dt><dd>32</dd>", "<dt>Tier</dt><dd>7</dd>", "Tier 2 · The Key", "--tier-max: ",
		`<meta name="description" content="Practice Go on real operations tasks, without an LLM. 50 exercises in 10 tiers,`,
	} {
		if !strings.Contains(landing, want) {
			t.Errorf("the landing page lacks %q", want)
		}
	}

	dash := h.do(req{method: "GET", path: "/learn", as: admin}).Body.String()
	for _, want := range []string{"of 50 exercises pass", "Tier 10 · The Key Turns", `id="tier-10"`} {
		if !strings.Contains(dash, want) {
			t.Errorf("the dashboard lacks %q", want)
		}
	}
	// Tier 1 and the started tier 7 are open; the others are folded.
	for n := 1; n <= 10; n++ {
		open := strings.Contains(dash, `aria-controls="tier-`+itoa(int64(n))+`-log" data-fold>Hide`)
		if want := n == 1 || n == 7; open != want {
			t.Errorf("tier %d open = %v, want %v", n, open, want)
		}
	}

	adminPage := h.do(req{method: "GET", path: "/admin", as: admin}).Body.String()
	if !strings.Contains(adminPage, "0 of 50") {
		t.Error("the admin page does not count 50 exercises")
	}
}
