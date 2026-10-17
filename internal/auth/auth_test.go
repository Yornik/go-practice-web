package auth_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/auth"
	"github.com/yornik/go-practice-web/internal/auth/authtest"
	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

const publicURL = "https://go-learn.party"

// clock is a settable clock shared by the store and the service.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// mailbox is a mail.Sender that keeps every message.
type mailbox struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	return nil
}

func (m *mailbox) all() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message(nil), m.msgs...)
}

// find returns the newest message to `to` whose subject contains subject.
func (m *mailbox) find(to, subject string) (mail.Message, bool) {
	msgs := m.all()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == to && strings.Contains(msgs[i].Subject, subject) {
			return msgs[i], true
		}
	}
	return mail.Message{}, false
}

var linkRE = regexp.MustCompile(`https://go-learn\.party/[^\s"<>]+`)

// tokenFrom returns the token parameter of the first site link in msg.
func tokenFrom(t *testing.T, msg mail.Message) string {
	const param = "token"
	t.Helper()
	for _, l := range linkRE.FindAllString(msg.Text, -1) {
		u, err := url.Parse(l)
		if err != nil {
			continue
		}
		if v := u.Query().Get(param); v != "" {
			return v
		}
	}
	t.Fatalf("no link with %q in %q", param, msg.Text)
	return ""
}

type fixture struct {
	st    *store.Store
	svc   *auth.Service
	mails *mailbox
	clock *clock
}

var client = auth.Client{IP: "192.0.2.10", UserAgent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/140.0 Safari/537.36"}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	clk := &clock{t: time.Now().UTC()}
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"), store.WithClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mb := &mailbox{}
	svc, err := auth.New(st, mail.NewMailer(mb, publicURL), auth.Config{PublicURL: publicURL, Now: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Wait)
	return &fixture{st: st, svc: svc, mails: mb, clock: clk}
}

func optionsJSON(t *testing.T, b auth.Begin) []byte {
	t.Helper()
	opts, err := json.Marshal(b.Options)
	if err != nil {
		t.Fatal(err)
	}
	return opts
}

// signUp runs a whole sign-up with the authenticator.
func (f *fixture) signUp(t *testing.T, a *authtest.Authenticator, in auth.SignUp) (store.User, error) {
	t.Helper()
	ctx := t.Context()
	b, err := f.svc.BeginRegistration(ctx, in, client)
	if err != nil {
		return store.User{}, err
	}
	cred, _, err := a.Create(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	return f.svc.FinishRegistration(ctx, auth.Finish{Ceremony: b.Ceremony, Credential: cred}, client)
}

func (f *fixture) login(t *testing.T, a *authtest.Authenticator) (store.User, error) {
	t.Helper()
	b, err := f.svc.BeginLogin(t.Context(), false, client)
	if err != nil {
		return store.User{}, err
	}
	assertion, err := a.Get(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	return f.svc.FinishLogin(t.Context(), auth.Finish{Ceremony: b.Ceremony, Credential: assertion}, client)
}

// admin creates the first account through the setup link.
func (f *fixture) admin(t *testing.T) (store.User, *authtest.Authenticator) {
	t.Helper()
	link, err := f.svc.EnsureSetupLink(t.Context())
	if err != nil || link == "" {
		t.Fatalf("EnsureSetupLink = %q, %v", link, err)
	}
	a := authtest.New(publicURL)
	u, err := f.signUp(t, a, auth.SignUp{Setup: query(t, link, "token"), Username: "Admin", DisplayName: "Ada Admin", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return u, a
}

// learner joins with an invite from admin.
func (f *fixture) learner(t *testing.T, admin store.User, username, email string) (store.User, *authtest.Authenticator) {
	t.Helper()
	inv, err := f.svc.CreateInvite(t.Context(), admin, auth.InviteRequest{}, client)
	if err != nil {
		t.Fatal(err)
	}
	a := authtest.New(publicURL)
	u, err := f.signUp(t, a, auth.SignUp{Invite: query(t, inv.Link, "invite"), Username: username, DisplayName: "Lee " + username, Email: email})
	if err != nil {
		t.Fatal(err)
	}
	return u, a
}

// verify uses the newest verification link sent to addr.
func (f *fixture) verify(t *testing.T, addr string) store.User {
	t.Helper()
	f.svc.Wait()
	msg, ok := f.mails.find(addr, "Verify")
	if !ok {
		t.Fatalf("no verification email to %s in %v", addr, f.mails.all())
	}
	u, err := f.svc.VerifyEmail(t.Context(), tokenFrom(t, msg), client)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func query(t *testing.T, link, param string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get(param)
}

// wantCode checks that err is an *auth.Error with the code and status.
func wantCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	ae, ok := auth.AsError(err)
	if !ok {
		t.Fatalf("error = %v, want *auth.Error %s", err, code)
	}
	if ae.Status != status || ae.Code != code {
		t.Fatalf("error = %d %s (%v), want %d %s", ae.Status, ae.Code, ae, status, code)
	}
}

func TestSetupLink(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	first, err := f.svc.EnsureSetupLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, publicURL+"/setup?token=") {
		t.Fatalf("setup link = %q", first)
	}
	// A restart before setup logs a new link and kills the old one.
	second, err := f.svc.EnsureSetupLink(ctx)
	if err != nil || second == first {
		t.Fatalf("second link = %q, %v", second, err)
	}
	wantCode(t, f.svc.CheckSetupToken(ctx, query(t, first, "token")), http.StatusGone, "token_invalid")
	if err := f.svc.CheckSetupToken(ctx, query(t, second, "token")); err != nil {
		t.Fatalf("CheckSetupToken(second) = %v", err)
	}

	a := authtest.New(publicURL)
	u, err := f.signUp(t, a, auth.SignUp{Setup: query(t, second, "token"), Username: "ada", DisplayName: "Ada", Email: ""})
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin || u.Username != "ada" {
		t.Fatalf("setup user = %+v, want admin ada", u)
	}
	if link, err := f.svc.EnsureSetupLink(ctx); err != nil || link != "" {
		t.Fatalf("EnsureSetupLink with a user = %q, %v; want none", link, err)
	}
	_, err = f.signUp(t, authtest.New(publicURL), auth.SignUp{Setup: query(t, second, "token"), Username: "eve", DisplayName: "Eve"})
	wantCode(t, err, http.StatusGone, "token_invalid")
}

func TestSignUpValidation(t *testing.T) {
	f := newFixture(t)
	admin, _ := f.admin(t)
	inv, err := f.svc.CreateInvite(t.Context(), admin, auth.InviteRequest{}, client)
	if err != nil {
		t.Fatal(err)
	}
	token := query(t, inv.Link, "invite")
	tests := []struct {
		name string
		in   auth.SignUp
		code string
	}{
		{"no token", auth.SignUp{Username: "bob", DisplayName: "Bob"}, "no_invite"},
		{"both tokens", auth.SignUp{Invite: token, Setup: token, Username: "bob", DisplayName: "Bob"}, "no_invite"},
		{"garbage token", auth.SignUp{Invite: "../../etc", Username: "bob", DisplayName: "Bob"}, "token_invalid"},
		{"short username", auth.SignUp{Invite: token, Username: "bo", DisplayName: "Bob"}, "bad_username"},
		{"username with space", auth.SignUp{Invite: token, Username: "bo b", DisplayName: "Bob"}, "bad_username"},
		{"taken username", auth.SignUp{Invite: token, Username: "ADMIN", DisplayName: "Bob"}, "username_taken"},
		{"empty name", auth.SignUp{Invite: token, Username: "bob", DisplayName: "  "}, "bad_display_name"},
		{"control in name", auth.SignUp{Invite: token, Username: "bob", DisplayName: "Bob\u202e"}, "bad_display_name"},
		{"bad email", auth.SignUp{Invite: token, Username: "bob", DisplayName: "Bob", Email: "bob"}, "bad_email"},
		{"email with name", auth.SignUp{Invite: token, Username: "bob", DisplayName: "Bob", Email: "Bob <bob@example.com>"}, "bad_email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.svc.BeginRegistration(t.Context(), tt.in, client)
			ae, ok := auth.AsError(err)
			if !ok || ae.Code != tt.code {
				t.Fatalf("BeginRegistration = %v, want code %s", err, tt.code)
			}
		})
	}
}

func TestInviteIsSingleUse(t *testing.T) {
	f := newFixture(t)
	admin, _ := f.admin(t)
	ctx := t.Context()
	inv, err := f.svc.CreateInvite(ctx, admin, auth.InviteRequest{Email: "bob@example.com", Note: "Welcome"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Mailed {
		t.Fatalf("invite not mailed: %+v", inv)
	}
	msg, ok := f.mails.find("bob@example.com", "invite")
	if !ok || !strings.Contains(msg.Text, inv.Link) {
		t.Fatalf("invite email = %+v, %v", msg, ok)
	}
	token := query(t, inv.Link, "invite")

	// 2 browsers start with the same invite; only the first to finish wins.
	a1, a2 := authtest.New(publicURL), authtest.New(publicURL)
	b1, err := f.svc.BeginRegistration(ctx, auth.SignUp{Invite: token, Username: "bob", DisplayName: "Bob", Email: "bob@example.com"}, client)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := f.svc.BeginRegistration(ctx, auth.SignUp{Invite: token, Username: "mallory", DisplayName: "Mal"}, client)
	if err != nil {
		t.Fatal(err)
	}
	c1, _, _ := a1.Create(optionsJSON(t, b1))
	c2, _, _ := a2.Create(optionsJSON(t, b2))
	bob, err := f.svc.FinishRegistration(ctx, auth.Finish{Ceremony: b1.Ceremony, Credential: c1}, client)
	if err != nil {
		t.Fatal(err)
	}
	if bob.IsAdmin || bob.Email != "bob@example.com" || bob.EmailVerified() {
		t.Fatalf("bob = %+v", bob)
	}
	_, err = f.svc.FinishRegistration(ctx, auth.Finish{Ceremony: b2.Ceremony, Credential: c2}, client)
	wantCode(t, err, http.StatusGone, "token_invalid")
	if _, err := f.st.UserByUsername(ctx, "mallory"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("mallory exists after a used invite: %v", err)
	}
	// A third attempt fails before the passkey prompt.
	_, err = f.svc.BeginRegistration(ctx, auth.SignUp{Invite: token, Username: "carol", DisplayName: "Carol"}, client)
	wantCode(t, err, http.StatusGone, "token_invalid")
	// A ceremony is single use too.
	_, err = f.svc.FinishRegistration(ctx, auth.Finish{Ceremony: b1.Ceremony, Credential: c1}, client)
	wantCode(t, err, http.StatusBadRequest, "ceremony_expired")

	// The sign-up sent a verification email.
	if u := f.verify(t, "bob@example.com"); !u.EmailVerified() {
		t.Fatalf("bob not verified: %+v", u)
	}
}

func TestInviteExpires(t *testing.T) {
	f := newFixture(t)
	admin, _ := f.admin(t)
	inv, err := f.svc.CreateInvite(t.Context(), admin, auth.InviteRequest{Days: 1}, client)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Hour)
	_, err = f.svc.BeginRegistration(t.Context(), auth.SignUp{Invite: query(t, inv.Link, "invite"), Username: "bob", DisplayName: "Bob"}, client)
	wantCode(t, err, http.StatusGone, "token_invalid")

	for _, days := range []int{-1, 31} {
		_, err := f.svc.CreateInvite(t.Context(), admin, auth.InviteRequest{Days: days}, client)
		wantCode(t, err, http.StatusBadRequest, "bad_days")
	}
}

func TestInviteNeedsAdmin(t *testing.T) {
	f := newFixture(t)
	admin, _ := f.admin(t)
	bob, _ := f.learner(t, admin, "bob", "")
	_, err := f.svc.CreateInvite(t.Context(), bob, auth.InviteRequest{}, client)
	wantCode(t, err, http.StatusForbidden, "forbidden")
}

func TestLoginAndSignCounter(t *testing.T) {
	f := newFixture(t)
	admin, a := f.admin(t)
	cred := a.Credentials()[0]
	cred.CountStep = 1 // a hardware key that counts

	for i := range 2 {
		u, err := f.login(t, a)
		if err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
		if u.ID != admin.ID {
			t.Fatalf("login %d: user %d, want %d", i, u.ID, admin.ID)
		}
	}
	stored, err := f.st.Credential(t.Context(), cred.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SignCount != 2 || stored.CloneWarning || stored.LastUsedAt.IsZero() {
		t.Fatalf("after 2 logins: %+v", stored)
	}

	// A copy of the key that lags behind reports an old counter.
	b, err := f.svc.BeginLogin(t.Context(), false, client)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := a.Replay(optionsJSON(t, b), cred, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.FinishLogin(t.Context(), auth.Finish{Ceremony: b.Ceremony, Credential: replay}, client)
	wantCode(t, err, http.StatusForbidden, "passkey_cloned")
	if stored, _ := f.st.Credential(t.Context(), cred.ID); !stored.CloneWarning {
		t.Fatalf("clone warning not stored: %+v", stored)
	}
}

func TestSyncedPasskeyCounterZero(t *testing.T) {
	f := newFixture(t)
	_, a := f.admin(t)
	// Synced passkeys always report 0; that is not a clone.
	for range 3 {
		if _, err := f.login(t, a); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConditionalLoginOptions(t *testing.T) {
	f := newFixture(t)
	b, err := f.svc.BeginLogin(t.Context(), true, client)
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Mediation string `json:"mediation"`
		PublicKey struct {
			RPID             string `json:"rpId"`
			Challenge        string `json:"challenge"`
			UserVerification string `json:"userVerification"`
			AllowCredentials []any  `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(optionsJSON(t, b), &o); err != nil {
		t.Fatal(err)
	}
	if o.Mediation != "conditional" || o.PublicKey.RPID != "go-learn.party" || o.PublicKey.Challenge == "" ||
		len(o.PublicKey.AllowCredentials) != 0 || o.PublicKey.UserVerification != "preferred" {
		t.Fatalf("conditional options = %+v", o)
	}
}

func TestRegistrationOptions(t *testing.T) {
	f := newFixture(t)
	link, _ := f.svc.EnsureSetupLink(t.Context())
	b, err := f.svc.BeginRegistration(t.Context(), auth.SignUp{Setup: query(t, link, "token"), Username: "ada", DisplayName: "Ada"}, client)
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		PublicKey struct {
			RP struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"rp"`
			User struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"user"`
			Attestation            string `json:"attestation"`
			AuthenticatorSelection struct {
				ResidentKey      string `json:"residentKey"`
				UserVerification string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(optionsJSON(t, b), &o); err != nil {
		t.Fatal(err)
	}
	pk := o.PublicKey
	if pk.RP.ID != "go-learn.party" || pk.RP.Name != "Go Practice" || pk.User.Name != "ada" || len(pk.User.ID) != 43 ||
		pk.Attestation != "none" || pk.AuthenticatorSelection.ResidentKey != "required" ||
		pk.AuthenticatorSelection.UserVerification != "preferred" {
		t.Fatalf("registration options = %+v", pk)
	}
}

func TestLoginWrongOrigin(t *testing.T) {
	f := newFixture(t)
	_, a := f.admin(t)
	a.Origin = "https://evil.example"
	_, err := f.login(t, a)
	wantCode(t, err, http.StatusBadRequest, "passkey_rejected")
}

func TestDisabledUser(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	admin, _ := f.admin(t)
	bob, a := f.learner(t, admin, "bob", "")

	rec := httptest.NewRecorder()
	if err := f.svc.StartSession(ctx, rec, bob.ID, client); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]

	disabled := true
	if err := f.svc.UpdateUser(ctx, admin, bob.ID, auth.UserUpdate{Disabled: &disabled}, client); err != nil {
		t.Fatal(err)
	}
	_, err := f.login(t, a)
	wantCode(t, err, http.StatusForbidden, "account_disabled")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	if _, err := f.svc.Authenticate(ctx, httptest.NewRecorder(), req); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("Authenticate after disable = %v, want ErrNoSession", err)
	}

	// Admins cannot lock themselves out.
	err = f.svc.UpdateUser(ctx, admin, admin.ID, auth.UserUpdate{Disabled: &disabled}, client)
	wantCode(t, err, http.StatusBadRequest, "self")
	notAdmin := false
	err = f.svc.UpdateUser(ctx, admin, admin.ID, auth.UserUpdate{Admin: &notAdmin}, client)
	wantCode(t, err, http.StatusBadRequest, "self")
	err = f.svc.UpdateUser(ctx, bob, admin.ID, auth.UserUpdate{Disabled: &disabled}, client)
	wantCode(t, err, http.StatusForbidden, "forbidden")

	enabled := false
	if err := f.svc.UpdateUser(ctx, admin, bob.ID, auth.UserUpdate{Disabled: &enabled}, client); err != nil {
		t.Fatal(err)
	}
	if _, err := f.login(t, a); err != nil {
		t.Fatalf("login after enable: %v", err)
	}
}

func TestAddRenameDeletePasskey(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	_, laptop := f.admin(t)
	admin := f.verify(t, "ada@example.com")

	// The laptop holds a passkey already: the browser refuses a duplicate.
	b, err := f.svc.BeginAddPasskey(ctx, admin, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := laptop.Create(optionsJSON(t, b)); !errors.Is(err, authtest.ErrExcluded) {
		t.Fatalf("Create on the same authenticator = %v, want ErrExcluded", err)
	}

	phone := authtest.New(publicURL)
	b, err = f.svc.BeginAddPasskey(ctx, admin, client)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := phone.Create(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	// Another user cannot finish the ceremony.
	bob, _ := f.learner(t, admin, "bob", "")
	_, err = f.svc.FinishAddPasskey(ctx, bob, auth.AddPasskey{Finish: auth.Finish{Ceremony: b.Ceremony, Credential: raw}}, client)
	wantCode(t, err, http.StatusBadRequest, "ceremony_expired")

	b, _ = f.svc.BeginAddPasskey(ctx, admin, client)
	raw, pc, err := phone.Create(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	added, err := f.svc.FinishAddPasskey(ctx, admin, auth.AddPasskey{Finish: auth.Finish{Ceremony: b.Ceremony, Credential: raw}, Name: "Phone"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if added.Name != "Phone" || string(added.ID) != string(pc.ID) {
		t.Fatalf("added = %+v", added)
	}
	f.svc.Wait()
	if _, ok := f.mails.find("ada@example.com", "passkey"); !ok {
		t.Fatalf("no passkey-added notice in %v", f.mails.all())
	}
	if u, err := f.login(t, phone); err != nil || u.ID != admin.ID {
		t.Fatalf("login with the new passkey = %v, %v", u, err)
	}

	if err := f.svc.RenamePasskey(ctx, admin, pc.ID, "  Work   phone ", client); err != nil {
		t.Fatal(err)
	}
	if c, _ := f.st.Credential(ctx, pc.ID); c.Name != "Work phone" {
		t.Fatalf("renamed = %q", c.Name)
	}
	wantCode(t, f.svc.RenamePasskey(ctx, admin, pc.ID, "", client), http.StatusBadRequest, "bad_passkey_name")
	wantCode(t, f.svc.RenamePasskey(ctx, bob, pc.ID, "Mine", client), http.StatusNotFound, "not_found")

	wantCode(t, f.svc.DeletePasskey(ctx, bob, pc.ID, client), http.StatusNotFound, "not_found")
	if err := f.svc.DeletePasskey(ctx, admin, laptop.Credentials()[0].ID, client); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.DeletePasskey(ctx, admin, pc.ID, client), http.StatusConflict, "last_passkey")
	// The removed passkey is still on the laptop but no longer works.
	_, err = f.login(t, laptop)
	wantCode(t, err, http.StatusBadRequest, "unknown_passkey")
}

func TestRecovery(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	admin, _ := f.admin(t)
	bob, old := f.learner(t, admin, "bob", "bob@example.com")
	f.verify(t, "bob@example.com")
	rec := httptest.NewRecorder()
	if err := f.svc.StartSession(ctx, rec, bob.ID, client); err != nil {
		t.Fatal(err)
	}

	f.learner(t, admin, "carol", "carol@example.com") // never verified

	// Unknown and unverified addresses get the same answer and no mail.
	f.svc.Wait()
	before := len(f.mails.all())
	for _, addr := range []string{"nobody@example.com", "carol@example.com", "BOB@example.com"} {
		if err := f.svc.RequestRecovery(ctx, addr, auth.Client{IP: "192.0.2.50"}); err != nil {
			t.Fatalf("RequestRecovery(%s) = %v", addr, err)
		}
	}
	f.svc.Wait()
	msgs := f.mails.all()
	if len(msgs) != before+1 || msgs[len(msgs)-1].To != "bob@example.com" {
		t.Fatalf("recovery mails = %v", msgs[before:])
	}
	token := tokenFrom(t, msgs[len(msgs)-1])

	u, err := f.svc.RecoveryUser(ctx, token)
	if err != nil || u.ID != bob.ID {
		t.Fatalf("RecoveryUser = %v, %v", u, err)
	}
	newKey := authtest.New(publicURL)
	b, err := f.svc.BeginRecovery(ctx, token, client)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := newKey.Create(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	u, err = f.svc.FinishRecovery(ctx, auth.Recover{Finish: auth.Finish{Ceremony: b.Ceremony, Credential: raw}, RemoveOthers: true}, client)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != bob.ID {
		t.Fatalf("recovered user %d, want %d", u.ID, bob.ID)
	}
	creds, _ := f.st.Credentials(ctx, bob.ID)
	if len(creds) != 1 {
		t.Fatalf("passkeys after recovery with removal = %d, want 1", len(creds))
	}
	if sessions, _ := f.st.Sessions(ctx, bob.ID); len(sessions) != 0 {
		t.Fatalf("sessions after recovery with removal = %d, want 0", len(sessions))
	}
	if _, err := f.login(t, old); err == nil {
		t.Fatal("the old passkey still works after recovery removed it")
	}
	if u, err := f.login(t, newKey); err != nil || u.ID != bob.ID {
		t.Fatalf("login with the recovered passkey = %v, %v", u, err)
	}
	f.svc.Wait()
	if _, ok := f.mails.find("bob@example.com", "recovered"); !ok {
		t.Fatalf("no recovery-used notice in %v", f.mails.all())
	}

	// The link works once.
	_, err = f.svc.BeginRecovery(ctx, token, client)
	wantCode(t, err, http.StatusGone, "token_invalid")
}

func TestRecoveryExpires(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.admin(t)
	f.verify(t, "ada@example.com")
	if err := f.svc.RequestRecovery(ctx, "ada@example.com", client); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	msg, ok := f.mails.find("ada@example.com", "Recover")
	if !ok {
		t.Fatalf("no recovery email in %v", f.mails.all())
	}
	token := tokenFrom(t, msg)
	f.clock.Advance(auth.RecoveryTTL + time.Second)
	_, err := f.svc.BeginRecovery(ctx, token, client)
	wantCode(t, err, http.StatusGone, "token_invalid")
}

func TestRecoveryRateLimit(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	// 3 per address per hour, from different addresses.
	for i := range 3 {
		if err := f.svc.RequestRecovery(ctx, "x@example.com", auth.Client{IP: fmt.Sprintf("192.0.2.%d", i+1)}); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	err := f.svc.RequestRecovery(ctx, "X@example.com", auth.Client{IP: "192.0.2.9"})
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	// 5 per IP per hour.
	for i := range 5 {
		err := f.svc.RequestRecovery(ctx, fmt.Sprintf("user%d@example.com", i), auth.Client{IP: "198.51.100.1"})
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	err = f.svc.RequestRecovery(ctx, "z@example.com", auth.Client{IP: "198.51.100.1"})
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	f.clock.Advance(time.Hour)
	if err := f.svc.RequestRecovery(ctx, "z@example.com", auth.Client{IP: "198.51.100.1"}); err != nil {
		t.Fatalf("after an hour: %v", err)
	}
}

func TestBeginRateLimit(t *testing.T) {
	f := newFixture(t)
	for i := range 20 {
		if _, err := f.svc.BeginLogin(t.Context(), false, client); err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
	}
	_, err := f.svc.BeginLogin(t.Context(), false, client)
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	if ae, _ := auth.AsError(err); ae.RetryAfter <= 0 {
		t.Fatalf("RetryAfter = %v", ae.RetryAfter)
	}
	if _, err := f.svc.BeginLogin(t.Context(), false, auth.Client{IP: "192.0.2.99"}); err != nil {
		t.Fatalf("another IP: %v", err)
	}
}

func TestBeginRateLimitCountsIPv6PerPrefix(t *testing.T) {
	f := newFixture(t)
	// An IPv6 client can pick any address of its /64; each new one must
	// not get a new budget.
	for i := range 20 {
		c := auth.Client{IP: fmt.Sprintf("2001:db8:7:7::%x", i+1)}
		if _, err := f.svc.BeginLogin(t.Context(), false, c); err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
	}
	_, err := f.svc.BeginLogin(t.Context(), false, auth.Client{IP: "2001:db8:7:7:abcd::1"})
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	err = f.svc.RequestRecovery(t.Context(), "a@example.com", auth.Client{IP: "2001:db8:7:8::1"})
	if err != nil {
		t.Fatalf("recovery from another /64: %v", err)
	}
	for i := range 4 {
		if err := f.svc.RequestRecovery(t.Context(), fmt.Sprintf("u%d@example.com", i), auth.Client{IP: fmt.Sprintf("2001:db8:7:8::%x", i+2)}); err != nil {
			t.Fatalf("recovery %d: %v", i, err)
		}
	}
	err = f.svc.RequestRecovery(t.Context(), "z@example.com", auth.Client{IP: "2001:db8:7:8::ffff"})
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
}

func TestVerifyEmailToken(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.admin(t)
	f.svc.Wait()
	msg, ok := f.mails.find("ada@example.com", "Verify")
	if !ok {
		t.Fatal("no verification email")
	}
	token := tokenFrom(t, msg)
	f.clock.Advance(auth.VerifyTTL + time.Minute)
	_, err := f.svc.VerifyEmail(ctx, token, client)
	wantCode(t, err, http.StatusGone, "token_invalid")
	_, err = f.svc.VerifyEmail(ctx, "not-a-token", client)
	wantCode(t, err, http.StatusGone, "token_invalid")
}

func TestChangeEmail(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.admin(t)
	admin := f.verify(t, "ada@example.com")

	res, err := f.svc.UpdateAccount(ctx, admin, auth.Account{DisplayName: "Ada L.", Email: "ada@new.example"}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !res.VerificationSent || res.User.DisplayName != "Ada L." || res.User.Email != "ada@example.com" {
		t.Fatalf("UpdateAccount = %+v", res)
	}
	f.svc.Wait()
	if _, ok := f.mails.find("ada@example.com", "changing"); !ok {
		t.Fatalf("no notice to the old address in %v", f.mails.all())
	}
	if pending, _ := f.svc.PendingEmail(ctx, res.User); pending != "ada@new.example" {
		t.Fatalf("pending = %q", pending)
	}
	// Until verified, recovery still uses the old address.
	if u, err := f.st.UserByVerifiedEmail(ctx, "ada@example.com"); err != nil || u.ID != admin.ID {
		t.Fatalf("old address lookup = %v, %v", u, err)
	}
	u := f.verify(t, "ada@new.example")
	if u.Email != "ada@new.example" || !u.EmailVerified() {
		t.Fatalf("after verify: %+v", u)
	}
	f.svc.Wait()
	if _, ok := f.mails.find("ada@new.example", "now your"); !ok {
		t.Fatalf("no notice to the new address in %v", f.mails.all())
	}

	_, err = f.svc.UpdateAccount(ctx, u, auth.Account{DisplayName: "Ada", Email: ""}, client)
	wantCode(t, err, http.StatusBadRequest, "email_required")

	bob, _ := f.learner(t, admin, "bob", "")
	_, err = f.svc.UpdateAccount(ctx, bob, auth.Account{DisplayName: "Bob", Email: "ADA@new.example"}, client)
	wantCode(t, err, http.StatusConflict, "email_taken")
}

func TestSessions(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	admin, _ := f.admin(t)
	rec := httptest.NewRecorder()
	if err := f.svc.StartSession(ctx, rec, admin.ID, client); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != "__Host-gpw_session" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int(auth.SessionTTL.Seconds()) {
		t.Fatalf("session cookie = %+v", cookie)
	}
	authenticate := func() (auth.Identity, *httptest.ResponseRecorder, error) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		id, err := f.svc.Authenticate(ctx, w, req)
		return id, w, err
	}
	id, w, err := authenticate()
	if err != nil || id.User.ID != admin.ID {
		t.Fatalf("Authenticate = %+v, %v", id, err)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("a fresh session was refreshed")
	}
	// After the refresh interval the session slides and the cookie renews.
	f.clock.Advance(auth.SessionRefresh + time.Minute)
	if _, w, err = authenticate(); err != nil || len(w.Result().Cookies()) != 1 {
		t.Fatalf("refresh: %v, cookies %v", err, w.Result().Cookies())
	}
	// 29 days later the slid session is still valid; 31 days idle is not.
	f.clock.Advance(29 * 24 * time.Hour)
	if _, _, err = authenticate(); err != nil {
		t.Fatalf("after 29 days: %v", err)
	}
	f.clock.Advance(31 * 24 * time.Hour)
	if _, w, err = authenticate(); !errors.Is(err, auth.ErrNoSession) || w.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("expired session = %v, cookies %v", err, w.Result().Cookies())
	}

	// A tampered cookie is no session.
	cookie.Value = "x" + cookie.Value[1:]
	if _, _, err = authenticate(); !errors.Is(err, auth.ErrNoSession) {
		t.Fatalf("tampered cookie = %v", err)
	}
}

func TestLogout(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	admin, _ := f.admin(t)
	cookies := make([]*http.Cookie, 0, 3)
	for range 3 {
		rec := httptest.NewRecorder()
		if err := f.svc.StartSession(ctx, rec, admin.ID, client); err != nil {
			t.Fatal(err)
		}
		cookies = append(cookies, rec.Result().Cookies()[0])
	}
	identity := func(c *http.Cookie) (auth.Identity, error) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.AddCookie(c)
		return f.svc.Authenticate(ctx, httptest.NewRecorder(), req)
	}
	id, _ := identity(cookies[0])
	if err := f.svc.Logout(ctx, httptest.NewRecorder(), id, false, client); err != nil {
		t.Fatal(err)
	}
	if _, err := identity(cookies[0]); !errors.Is(err, auth.ErrNoSession) {
		t.Fatal("session alive after logout")
	}
	id, err := identity(cookies[1])
	if err != nil {
		t.Fatal("logout ended another session")
	}
	if err := f.svc.Logout(ctx, httptest.NewRecorder(), id, true, client); err != nil {
		t.Fatal(err)
	}
	if _, err := identity(cookies[2]); !errors.Is(err, auth.ErrNoSession) {
		t.Fatal("session alive after logout everywhere")
	}
}

func TestDevMode(t *testing.T) {
	const devURL = "http://localhost:8080"
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "dev.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	mailer := mail.NewMailer(&mailbox{}, devURL)
	if _, err := auth.New(st, mailer, auth.Config{PublicURL: devURL}); err == nil {
		t.Fatal("http public URL accepted outside dev mode")
	}
	svc, err := auth.New(st, mailer, auth.Config{PublicURL: devURL, Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Wait()
	link, err := svc.EnsureSetupLink(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.BeginRegistration(t.Context(), auth.SignUp{Setup: query(t, link, "token"), Username: "dev", DisplayName: "Dev"}, client)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := authtest.New(devURL).Create(optionsJSON(t, b))
	if err != nil {
		t.Fatal(err)
	}
	u, err := svc.FinishRegistration(t.Context(), auth.Finish{Ceremony: b.Ceremony, Credential: raw}, client)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := svc.StartSession(t.Context(), rec, u.ID, client); err != nil {
		t.Fatal(err)
	}
	c := rec.Result().Cookies()[0]
	if c.Name != "gpw_session" || c.Secure {
		t.Fatalf("dev cookie = %+v", c)
	}
}

func TestRevokeInvite(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	admin, _ := f.admin(t)
	inv, err := f.svc.CreateInvite(ctx, admin, auth.InviteRequest{}, client)
	if err != nil {
		t.Fatal(err)
	}
	hashHex := hex.EncodeToString(inv.Invite.TokenHash)
	if err := f.svc.RevokeInvite(ctx, admin, hashHex, client); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.RevokeInvite(ctx, admin, hashHex, client), http.StatusNotFound, "not_found")
	wantCode(t, f.svc.RevokeInvite(ctx, admin, "zz", client), http.StatusNotFound, "not_found")
	_, err = f.svc.CheckInvite(ctx, query(t, inv.Link, "invite"))
	wantCode(t, err, http.StatusGone, "token_invalid")
}

func TestEmailChangesAreRateLimited(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.admin(t)
	admin := f.verify(t, "ada@example.com")
	// Each change mails a link to the new address; a script with a session
	// must not turn the site into a mailer for any address it likes.
	for i := range 5 {
		if _, err := f.svc.UpdateAccount(ctx, admin, auth.Account{DisplayName: "Ada", Email: fmt.Sprintf("victim%d@example.net", i)}, client); err != nil {
			t.Fatalf("change %d: %v", i, err)
		}
	}
	_, err := f.svc.UpdateAccount(ctx, admin, auth.Account{DisplayName: "Eve", Email: "victim9@example.net"}, client)
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	if u, _ := f.st.User(ctx, admin.ID); u.DisplayName != "Ada" {
		t.Errorf("a refused change still set the name to %q", u.DisplayName)
	}
	_, err = f.svc.ResendVerification(ctx, admin, client)
	wantCode(t, err, http.StatusTooManyRequests, "rate_limited")
	// Keeping the verified address mails nothing and is not limited.
	if _, err := f.svc.UpdateAccount(ctx, admin, auth.Account{DisplayName: "Ada L.", Email: "ada@example.com"}, client); err != nil {
		t.Fatalf("name change: %v", err)
	}
	f.clock.Advance(time.Hour)
	if _, err := f.svc.UpdateAccount(ctx, admin, auth.Account{DisplayName: "Ada", Email: "ada@other.example"}, client); err != nil {
		t.Fatalf("change after an hour: %v", err)
	}
}
