package web_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/cert"
)

func TestCertificateClaimAndVerify(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	claim := func(body map[string]any) (int, map[string]string) {
		w := h.do(req{method: "POST", path: "/api/certificates", body: body, as: admin})
		if w.Code == http.StatusCreated {
			return w.Code, decodeJSON[map[string]string](t, w)
		}
		return w.Code, decodeJSON[map[string]string](t, w)
	}

	if code, body := claim(map[string]any{"kind": "tier1"}); code != http.StatusForbidden || body["code"] != "not_eligible" {
		t.Fatalf("claim without passes = %d %v", code, body)
	}
	if code, body := claim(map[string]any{"kind": "tier9"}); code != http.StatusBadRequest || body["code"] != "bad_kind" {
		t.Fatalf("claim of tier9 = %d %v", code, body)
	}
	tier1, _ := testCourse.Tier(1)
	for _, e := range tier1.Exercises[:len(tier1.Exercises)-1] {
		if _, err := h.store.MarkPassed(t.Context(), admin.ID, e.Slug); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := claim(map[string]any{"kind": "tier1"}); code != http.StatusForbidden {
		t.Fatalf("claim with 1 exercise missing = %d", code)
	}
	if _, err := h.store.MarkPassed(t.Context(), admin.ID, tier1.Exercises[len(tier1.Exercises)-1].Slug); err != nil {
		t.Fatal(err)
	}
	if code, body := claim(map[string]any{"kind": "tier1", "name": "Ada 😀"}); code != http.StatusBadRequest || body["code"] != "bad_name" {
		t.Fatalf("claim with an emoji = %d %v", code, body)
	}
	code, first := claim(map[string]any{"kind": "tier1", "name": "Ada Lovelace", "showStats": true})
	if code != http.StatusCreated || !strings.HasPrefix(first["id"], "GP-") || first["url"] != "/verify/"+first["id"] {
		t.Fatalf("claim = %d %v", code, first)
	}

	// The verify page shows what was signed.
	w := h.do(req{method: "GET", path: first["url"]})
	wantStatus(t, w, http.StatusOK)
	page := w.Body.String()
	for _, want := range []string{"Ada Lovelace", "Valid certificate", h.signer.Fingerprint(), tier1.Exercises[0].Title, "Statistics"} {
		if !strings.Contains(page, want) {
			t.Errorf("verify page lacks %q", want)
		}
	}
	w = h.do(req{method: "GET", path: first["pdf"]})
	wantStatus(t, w, http.StatusOK)
	if w.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("pdf = %s %q", w.Header().Get("Content-Type"), w.Body.String()[:20])
	}

	// Lower case and missing hyphens redirect to the canonical page.
	w = h.do(req{method: "GET", path: "/verify/" + strings.ToLower(strings.ReplaceAll(first["id"], "-", ""))})
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != first["url"] {
		t.Fatalf("lower-case ID = %d %s", w.Code, w.Header().Get("Location"))
	}

	// Claiming again replaces the certificate and revokes the old one.
	code, second := claim(map[string]any{"kind": "tier1", "name": "Ada King"})
	if code != http.StatusCreated || second["replaced"] != first["id"] {
		t.Fatalf("re-claim = %d %v", code, second)
	}
	w = h.do(req{method: "GET", path: first["url"]})
	wantStatus(t, w, http.StatusOK)
	if page := w.Body.String(); !strings.Contains(page, "Revoked") || !strings.Contains(page, second["id"]) {
		t.Fatalf("revoked page lacks the replacement:\n%s", page)
	}
	if page := h.do(req{method: "GET", path: second["url"]}).Body.String(); strings.Contains(page, "Statistics") {
		t.Fatal("stats shown although not chosen")
	}

	list := decodeJSON[struct {
		Kinds []struct {
			Kind        string `json:"kind"`
			Eligible    bool   `json:"eligible"`
			Certificate *struct {
				ID string `json:"id"`
			} `json:"certificate"`
		} `json:"kinds"`
	}](t, h.do(req{method: "GET", path: "/api/certificates", as: admin}))
	if len(list.Kinds) != 6 || list.Kinds[0].Certificate == nil || list.Kinds[0].Certificate.ID != second["id"] || list.Kinds[1].Eligible {
		t.Fatalf("certificates = %+v", list)
	}
}

func TestVerifyUnknownAndBad(t *testing.T) {
	h := newHarness(t)
	id := cert.NewID()
	for _, p := range []string{"/verify/" + id, "/verify/not-an-id", "/verify/" + id + ".pdf"} {
		if w := h.do(req{method: "GET", path: p}); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, w.Code)
		}
	}
	w := h.do(req{method: "GET", path: "/verify?id=" + strings.ToLower(id)})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/verify/"+id {
		t.Fatalf("lookup = %d %s", w.Code, w.Header().Get("Location"))
	}
	w = h.do(req{method: "GET", path: "/verify?id=nonsense"})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), "not a certificate ID") {
		t.Fatal("lookup error not shown")
	}
	w = h.do(req{method: "GET", path: "/.well-known/go-practice-certificates.pub"})
	wantStatus(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != h.signer.PublicKeyBase64() {
		t.Fatalf("public key = %q", w.Body.String())
	}
}
