package web_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/auth/authtest"
	"github.com/yornik/go-practice-web/internal/mail"
)

var linkRE = regexp.MustCompile(`https://go-learn\.party/[^\s"<>]+`)

// lastLink returns the token of the newest mailed link to addr whose
// subject contains subject.
func (h *harness) lastLink(addr, subject string) string {
	h.t.Helper()
	h.auth.Wait()
	h.mails.mu.Lock()
	defer h.mails.mu.Unlock()
	for i := len(h.mails.msgs) - 1; i >= 0; i-- {
		m := h.mails.msgs[i]
		if m.To != addr || !strings.Contains(m.Subject, subject) {
			continue
		}
		return tokenOf(h.t, m)
	}
	h.t.Fatalf("no %q mail to %s", subject, addr)
	return ""
}

func tokenOf(t *testing.T, m mail.Message) string {
	t.Helper()
	for _, l := range linkRE.FindAllString(m.Text, -1) {
		if u, err := url.Parse(l); err == nil && u.Query().Get("token") != "" {
			return u.Query().Get("token")
		}
	}
	t.Fatalf("no token link in %q", m.Text)
	return ""
}

func TestVerifyEmailAndRecoveryAPI(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()

	token := h.lastLink("ada@example.com", "Verify")
	w := h.do(req{method: "POST", path: "/api/auth/verify-email", body: map[string]string{"token": token}})
	wantStatus(t, w, http.StatusOK)
	w = h.do(req{method: "POST", path: "/api/auth/verify-email", body: map[string]string{"token": token}})
	wantStatus(t, w, http.StatusGone)

	// The same answer for a known and an unknown address.
	var answers []string
	for _, addr := range []string{"ada@example.com", "nobody@example.com"} {
		w := h.do(req{method: "POST", path: "/api/auth/recover", body: map[string]string{"email": addr}})
		wantStatus(t, w, http.StatusOK)
		answers = append(answers, w.Body.String())
	}
	if answers[0] != answers[1] {
		t.Fatalf("recovery answers differ: %q vs %q", answers[0], answers[1])
	}
	token = h.lastLink("ada@example.com", "Recover")
	w = h.do(req{method: "GET", path: "/recover/confirm?token=" + token})
	wantStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), "Ada Admin") {
		t.Fatal("confirm page does not greet the user")
	}

	key := authtest.New(publicURL)
	w = h.do(req{method: "POST", path: "/api/auth/recover/begin", body: map[string]string{"token": token}})
	wantStatus(t, w, http.StatusOK)
	begin := decodeJSON[struct {
		Ceremony string          `json:"ceremony"`
		Options  json.RawMessage `json:"options"`
	}](t, w)
	cred, _, err := key.Create(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/auth/recover/finish", body: map[string]any{
		"ceremony": begin.Ceremony, "credential": cred, "removeOthers": true,
	}})
	wantStatus(t, w, http.StatusOK)
	recovered := h.userFrom(w, key)
	if recovered.ID != admin.ID {
		t.Fatalf("recovered user %d, want %d", recovered.ID, admin.ID)
	}
	// The old session ended with the other passkeys; the new one works.
	if w := h.do(req{method: "GET", path: "/api/certificates", as: admin}); w.Code != http.StatusUnauthorized {
		t.Fatalf("old session after recovery = %d", w.Code)
	}
	wantStatus(t, h.do(req{method: "GET", path: "/api/certificates", as: recovered}), http.StatusOK)
	wantStatus(t, h.do(req{method: "POST", path: "/api/auth/recover/begin", body: map[string]string{"token": token}}), http.StatusGone)
}

func TestAccountAndPasskeysAPI(t *testing.T) {
	h := newHarness(t)
	admin := h.admin()
	w := h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "Ada L.", "email": "ada@new.example"}, as: admin})
	wantStatus(t, w, http.StatusOK)
	res := decodeJSON[map[string]any](t, w)
	if res["verificationSent"] != true || res["pendingEmail"] != "ada@new.example" {
		t.Fatalf("account update = %v", res)
	}
	w = h.do(req{method: "PUT", path: "/api/account", body: map[string]string{"displayName": "", "email": ""}, as: admin})
	wantStatus(t, w, http.StatusBadRequest)

	// Add a second passkey, rename it, remove the first.
	w = h.do(req{method: "POST", path: "/api/auth/passkeys/begin", body: map[string]any{}, as: admin})
	wantStatus(t, w, http.StatusOK)
	begin := decodeJSON[struct {
		Ceremony string          `json:"ceremony"`
		Options  json.RawMessage `json:"options"`
	}](t, w)
	phone := authtest.New(publicURL)
	cred, _, err := phone.Create(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	w = h.do(req{method: "POST", path: "/api/auth/passkeys/finish", body: map[string]any{"ceremony": begin.Ceremony, "credential": cred, "name": "Phone"}, as: admin})
	wantStatus(t, w, http.StatusOK)
	added := decodeJSON[map[string]string](t, w)
	wantStatus(t, h.do(req{method: "PUT", path: "/api/auth/passkeys/" + added["id"], body: map[string]string{"name": "Work phone"}, as: admin}), http.StatusOK)
	firstID := base64.RawURLEncoding.EncodeToString(admin.key.Credentials()[0].ID)
	wantStatus(t, h.do(req{method: "DELETE", path: "/api/auth/passkeys/" + firstID, as: admin}), http.StatusOK)
	w = h.do(req{method: "DELETE", path: "/api/auth/passkeys/" + added["id"], as: admin})
	wantStatus(t, w, http.StatusConflict)
	wantStatus(t, h.do(req{method: "DELETE", path: "/api/auth/passkeys/!!", as: admin}), http.StatusNotFound)
	page := h.do(req{method: "GET", path: "/account", as: admin}).Body.String()
	if !strings.Contains(page, "Work phone") || strings.Contains(page, "data-passkey-name>Passkey<") {
		t.Fatal("account page does not show the passkeys")
	}
}
