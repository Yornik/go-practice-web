package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenderSamples writes every mail, both parts, for a look by eye next
// to .design/vesper-v2/email.html, when MAIL_SAMPLES_DIR is set:
//
//	MAIL_SAMPLES_DIR=/tmp/mails go test -run TestRenderSamples ./internal/mail/
func TestRenderSamples(t *testing.T) {
	dir := os.Getenv("MAIL_SAMPLES_DIR")
	if dir == "" {
		t.Skip("MAIL_SAMPLES_DIR not set")
	}
	const site = "https://vesper-labs.org"
	at := time.Date(2026, 10, 21, 3, 12, 0, 0, time.UTC)
	samples := []struct {
		to string
		e  Email
	}{
		{"ada@example.org", Invite{InvitedBy: "Yornik", Note: "Start with 01. Tier 1 takes about 7 hours.", Link: site + "/join?invite=Qm9vdHN0cmFwLXRva2VuLWZvci10aGUtZnJhbWU",
			Expires: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), ValidFor: 7 * 24 * time.Hour, Exercises: 50}},
		{"ada.lovelace@example.net", VerifyEmail{Name: "Ada Lovelace", Link: site + "/verify-email?token=dmVyaWZ5LXRva2VuLWZvci10aGUtZnJhbWUtb2YtdGhlLW1haWw", ValidFor: 48 * time.Hour}},
		{"anna@example.org", Recovery{Name: "Анна", Link: site + "/recover/confirm?token=cmVjb3ZlcnktdG9rZW4tZm9yLXRoZS1mcmFtZQ", ValidFor: 15 * time.Minute}},
		{"anna@example.org", RecoveryUsed{Name: "Анна", Passkey: "Firefox on Linux", When: at, RemovedOthers: 2}},
		{"ada.lovelace@example.net", PasskeyAdded{Name: "Ada Lovelace", Passkey: "New phone", When: at.Add(11 * time.Hour)}},
		{"ada.lovelace@example.net", PasskeyRemoved{Name: "Ada Lovelace", Passkey: "Old phone", When: at.Add(11*time.Hour + 8*time.Minute)}},
		{"ada@example.org", EmailChangedOld{EmailChange{Name: "Ada Lovelace", Old: "ada@example.org", New: "ada.lovelace@example.net", When: at, Pending: true}}},
		{"ada.lovelace@example.net", EmailChangedNew{EmailChange{Name: "Ada Lovelace", Old: "ada@example.org", New: "ada.lovelace@example.net", When: at}}},
	}
	for _, s := range samples {
		m, err := compose(site, "hush@vesper-labs.org", s.to, s.e)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Join(dir, s.e.templateName())
		if err := os.WriteFile(base+".html", []byte(m.HTML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(base+".txt", []byte("Subject: "+m.Subject+"\n\n"+m.Text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
