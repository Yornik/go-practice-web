package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// clock is a settable test clock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// t0 is a Monday.
var t0 = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

func newStore(t *testing.T) (*Store, *clock) {
	t.Helper()
	clk := &clock{now: t0}
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), WithClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, clk
}

var handleSeq atomic.Int64

func newUser(name string) NewUser {
	return NewUser{
		Username:       name,
		DisplayName:    strings.ToUpper(name[:1]) + name[1:],
		Email:          name + "@example.com",
		WebAuthnHandle: fmt.Appendf(nil, "handle-%d", handleSeq.Add(1)),
	}
}

func newCred(id string) Credential {
	return Credential{
		ID:              []byte(id),
		PublicKey:       []byte("pk-" + id),
		AttestationType: "none",
		AAGUID:          bytes.Repeat([]byte{1}, 16),
		SignCount:       0,
		Transports:      []string{"internal", "hybrid"},
		Attachment:      "platform",
		UserPresent:     true,
		UserVerified:    true,
		BackupEligible:  true,
		BackupState:     true,
		Name:            "Passkey " + id,
	}
}

// register creates a user through an invite.
func register(t *testing.T, s *Store, name string) User {
	t.Helper()
	ctx := context.Background()
	hash := []byte("invite-" + name)
	if _, err := s.CreateInvite(ctx, Invite{TokenHash: hash, ExpiresAt: s.now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	u, err := s.RegisterWithInvite(ctx, hash, Registration{User: newUser(name), Credential: newCred("cred-" + name)})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestOpenMigratesAndSetsPragmas(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.SchemaVersion(ctx)
	if err != nil || v != 3 {
		t.Fatalf("SchemaVersion = %d, %v", v, err)
	}
	// Every connection of the pool has the pragmas, not only the first.
	conns := make([]*sql.Conn, maxOpenConns)
	for i := range conns {
		if conns[i], err = s.db.Conn(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range conns {
		var journal string
		var fk, sync, busy int
		if err := c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&sync); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if journal != "delete" || fk != 1 || sync != 2 || busy != int(busyTimeout.Milliseconds()) {
			t.Errorf("conn %d: journal_mode=%s foreign_keys=%d synchronous=%d busy_timeout=%d", i, journal, fk, sync, busy)
		}
	}
	for _, c := range conns {
		_ = c.Close()
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening applies nothing twice.
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 3 {
		t.Errorf("schema_migrations has %d rows, %v", n, err)
	}
	// A schema from a newer build stops an older one.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (9999, 'future', 'x')`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if _, err := Open(ctx, path); err == nil || !strings.Contains(err.Error(), "9999") {
		t.Errorf("Open with an unknown migration: %v", err)
	}

	for _, bad := range []string{"", "a?b.db", "a#b.db"} {
		if _, err := Open(ctx, bad); err == nil {
			t.Errorf("Open(%q) succeeded", bad)
		}
	}
	if _, err := Open(ctx, filepath.Join(t.TempDir(), "missing", "dir", "x.db")); err == nil {
		t.Error("Open in a missing directory succeeded")
	}
}

// TestMigrationInviteMailed upgrades a database at schema version 1: its
// invites keep working and none of them counts as mailed.
func TestMigrationInviteMailed(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		t.Fatal(err)
	}
	initSQL, err := migrations.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`,
		string(initSQL),
		`INSERT INTO schema_migrations VALUES (1, 'init', '2026-10-01T00:00:00.000Z')`,
		`INSERT INTO invites (token_hash, email, created_at, expires_at) VALUES (x'01', 'bob@example.com', '2026-10-19T09:00:00.000Z', '2026-10-26T09:00:00.000Z')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%.40s: %v", q, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	clk := &clock{now: t0}
	s, err := Open(ctx, path, WithClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if v, err := s.SchemaVersion(ctx); err != nil || v != 3 {
		t.Fatalf("SchemaVersion = %d, %v", v, err)
	}
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE version = 2`).Scan(&name); err != nil || name != "invite_mailed" {
		t.Errorf("migration 2 = %q, %v", name, err)
	}
	inv, err := s.ValidInvite(ctx, []byte{1})
	if err != nil || inv.Mailed() || !inv.MailedAt.IsZero() || inv.Email != "bob@example.com" {
		t.Fatalf("old invite = %+v, %v", inv, err)
	}
	// An old invite was never marked, so its sign-up mails a link as before.
	u, err := s.RegisterWithInvite(ctx, []byte{1}, Registration{User: newUser("bob"), Credential: newCred("c1")})
	if err != nil || u.EmailVerified() {
		t.Fatalf("sign-up with an old invite = %+v, %v", u, err)
	}
}

func TestInviteMailedVerifiesTheAddress(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	tests := []struct {
		name    string
		invite  string // the invite's address
		mailed  bool
		signUp  string // the address of the sign-up
		wantVer bool
	}{
		{"mailed, same address", "dora@example.com", true, "dora@example.com", true},
		{"mailed, other case", " Erin@Example.COM ", true, "erin@example.com", true},
		{"mailed, other address", "fay@example.com", true, "fay@example.org", false},
		{"mailed, address with a suffix", "gus@example.com", true, "gus@example.com.evil", false},
		{"not mailed", "hal@example.com", false, "hal@example.com", false},
		{"copied link without address", "", false, "ida@example.com", false},
		// Only ASCII letters fold, as COLLATE NOCASE does: a Kelvin sign
		// or a long s is another mailbox, which no mail reached.
		{"kelvin sign", "kate@example.com", true, "Kate@example.com", false},
		{"long s", "sam@example.com", true, "ſam@example.com", false},
	}
	for i, tt := range tests {
		hash := fmt.Appendf(nil, "inv-%d", i)
		if _, err := s.CreateInvite(ctx, Invite{TokenHash: hash, Email: tt.invite, ExpiresAt: t0.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if tt.mailed {
			clk.Advance(time.Second)
			if err := s.MarkInviteMailed(ctx, hash); err != nil {
				t.Fatal(err)
			}
			inv, err := s.Invite(ctx, hash)
			if err != nil || !inv.Mailed() || !inv.MailedAt.Equal(clk.Now()) {
				t.Fatalf("%s: marked invite = %+v, %v", tt.name, inv, err)
			}
		}
		nu := newUser(fmt.Sprintf("user%d", i))
		nu.Email = tt.signUp
		u, err := s.RegisterWithInvite(ctx, hash, Registration{User: nu, Credential: newCred(fmt.Sprintf("c%d", i))})
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if u.EmailVerified() != tt.wantVer {
			t.Errorf("%s: verified = %v, want %v", tt.name, u.EmailVerified(), tt.wantVer)
		}
		// The stored row says the same as the returned user.
		got, err := s.User(ctx, u.ID)
		if err != nil || got.EmailVerified() != tt.wantVer || got.EmailVerifiedAt != u.EmailVerifiedAt {
			t.Errorf("%s: stored user = %+v, %v; returned %+v", tt.name, got, err, u)
		}
		if tt.wantVer && !u.EmailVerifiedAt.Equal(clk.Now()) {
			t.Errorf("%s: verified at %v, want %v", tt.name, u.EmailVerifiedAt, clk.Now())
		}
	}

	// A used or unknown invite cannot be marked; a failed registration
	// leaves the address unverified.
	if err := s.MarkInviteMailed(ctx, []byte("inv-0")); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkInviteMailed of a used invite: %v", err)
	}
	if err := s.MarkInviteMailed(ctx, []byte("nope")); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkInviteMailed of an unknown invite: %v", err)
	}
	if _, err := s.CreateInvite(ctx, Invite{TokenHash: []byte("taken"), Email: "jo@example.com", ExpiresAt: t0.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkInviteMailed(ctx, []byte("taken")); err != nil {
		t.Fatal(err)
	}
	nu := newUser("user0") // the username is taken
	nu.Email = "jo@example.com"
	if _, err := s.RegisterWithInvite(ctx, []byte("taken"), Registration{User: nu, Credential: newCred("c-jo")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("register with a taken username: %v", err)
	}
	if inv, err := s.ValidInvite(ctx, []byte("taken")); err != nil || !inv.Mailed() {
		t.Errorf("the invite after a failed sign-up = %+v, %v", inv, err)
	}
}

func TestRegisterWithInvite(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	admin := register(t, s, "admin")

	inv, err := s.CreateInvite(ctx, Invite{TokenHash: []byte("inv"), CreatedBy: admin.ID, Email: " Bob@Example.com ",
		Note: "for Bob", ExpiresAt: t0.Add(7 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if inv.CreatedAt != t0 {
		t.Errorf("CreatedAt = %v", inv.CreatedAt)
	}
	if _, err := s.ValidInvite(ctx, []byte("inv")); err != nil {
		t.Fatalf("ValidInvite: %v", err)
	}

	tests := []struct {
		name    string
		invite  string
		user    NewUser
		cred    string
		wantErr error
		column  string
	}{
		{"unknown invite", "nope", newUser("bob"), "c1", ErrTokenInvalid, ""},
		{"username taken", "inv", NewUser{Username: "admin", DisplayName: "Other", WebAuthnHandle: []byte("h-other")}, "c1", ErrConflict, "users.username"},
		{"email taken, other case", "inv", NewUser{Username: "bob", DisplayName: "Bob", Email: "ADMIN@example.com", WebAuthnHandle: []byte("h-bob")}, "c1", ErrConflict, "users.email"},
		{"credential taken", "inv", newUser("bob"), "cred-admin", ErrConflict, "credentials.id"},
		{"bad username", "inv", NewUser{Username: "Bob!", DisplayName: "Bob", WebAuthnHandle: []byte("h-bob2")}, "c1", nil, ""},
		{"ok", "inv", newUser("bob"), "c1", nil, ""},
		{"invite used", "inv", newUser("carol"), "c2", ErrTokenInvalid, ""},
	}
	for _, tt := range tests {
		u, err := s.RegisterWithInvite(ctx, []byte(tt.invite), Registration{User: tt.user, Credential: newCred(tt.cred)})
		switch {
		case tt.name == "bad username":
			if err == nil {
				t.Errorf("%s: registered %q", tt.name, tt.user.Username)
			}
		case tt.wantErr != nil:
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
			}
			var ce *ConflictError
			if tt.column != "" && (!errors.As(err, &ce) || ce.Column != tt.column) {
				t.Errorf("%s: err = %v, want a conflict on %s", tt.name, err, tt.column)
			}
		case err != nil:
			t.Errorf("%s: %v", tt.name, err)
		default:
			if u.ID == 0 || u.IsAdmin || u.EmailVerified() || u.Email != "bob@example.com" || u.CreatedAt != t0 {
				t.Errorf("%s: user = %+v", tt.name, u)
			}
		}
	}

	got, err := s.Invite(ctx, []byte("inv"))
	if err != nil || !got.Used() || got.UsedAt != t0 || got.CreatedBy != admin.ID || got.Email != "Bob@Example.com" {
		t.Errorf("used invite = %+v, %v", got, err)
	}
	bob, err := s.UserByUsername(ctx, "bob")
	if err != nil || got.UsedBy != bob.ID {
		t.Errorf("invite used by %d, bob is %d (%v)", got.UsedBy, bob.ID, err)
	}
	// A failed registration leaves nothing behind.
	if n, _ := s.CountUsers(ctx); n != 2 {
		t.Errorf("CountUsers = %d, want 2", n)
	}
	if _, err := s.Credential(ctx, []byte("c2")); !errors.Is(err, ErrNotFound) {
		t.Errorf("credential of a failed registration: %v", err)
	}

	// Expiry.
	if _, err := s.CreateInvite(ctx, Invite{TokenHash: []byte("short"), ExpiresAt: t0.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Minute)
	if _, err := s.ValidInvite(ctx, []byte("short")); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("ValidInvite of an expired invite: %v", err)
	}
	if _, err := s.RegisterWithInvite(ctx, []byte("short"), Registration{User: newUser("dave"), Credential: newCred("c3")}); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("RegisterWithInvite with an expired invite: %v", err)
	}

	// Listing and revoking.
	invites, err := s.Invites(ctx)
	if err != nil || len(invites) != 3 {
		t.Fatalf("Invites = %d, %v", len(invites), err)
	}
	if err := s.DeleteInvite(ctx, []byte("inv")); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteInvite of a used invite: %v", err)
	}
	if err := s.DeleteInvite(ctx, []byte("short")); err != nil {
		t.Errorf("DeleteInvite: %v", err)
	}
	if _, err := s.Invite(ctx, []byte("short")); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted invite: %v", err)
	}
}

func TestInviteRace(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.CreateInvite(ctx, Invite{TokenHash: []byte("once"), ExpiresAt: t0.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	const n = 8
	var wg sync.WaitGroup
	var ok, invalid atomic.Int32
	for i := range n {
		wg.Go(func() {
			_, err := s.RegisterWithInvite(ctx, []byte("once"), Registration{
				User:       newUser(fmt.Sprintf("user%d", i)),
				Credential: newCred(fmt.Sprintf("c%d", i)),
			})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrTokenInvalid):
				invalid.Add(1)
			default:
				t.Errorf("register %d: %v", i, err)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 1 || invalid.Load() != n-1 {
		t.Errorf("%d registrations succeeded and %d were refused, want 1 and %d", ok.Load(), invalid.Load(), n-1)
	}
}

func TestSetupToken(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if ok, err := s.HasUnusedSetupToken(ctx); err != nil || ok {
		t.Fatalf("HasUnusedSetupToken = %v, %v", ok, err)
	}
	if err := s.ReplaceSetupToken(ctx, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceSetupToken(ctx, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidSetupToken(ctx, []byte("old")); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("replaced setup token: %v", err)
	}
	if err := s.ValidSetupToken(ctx, []byte("new")); err != nil {
		t.Errorf("ValidSetupToken: %v", err)
	}
	r := Registration{User: newUser("root"), Credential: newCred("c-root")}
	if _, err := s.RegisterWithSetupToken(ctx, []byte("old"), r); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("RegisterWithSetupToken with a replaced token: %v", err)
	}
	u, err := s.RegisterWithSetupToken(ctx, []byte("new"), r)
	if err != nil || !u.IsAdmin {
		t.Fatalf("RegisterWithSetupToken = %+v, %v", u, err)
	}
	r2 := Registration{User: newUser("root2"), Credential: newCred("c-root2")}
	if _, err := s.RegisterWithSetupToken(ctx, []byte("new"), r2); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("second use of the setup token: %v", err)
	}
	if ok, _ := s.HasUnusedSetupToken(ctx); ok {
		t.Error("setup token unused after setup")
	}
}

func TestUsers(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	for name, get := range map[string]func() (User, error){
		"id":       func() (User, error) { return s.User(ctx, u.ID) },
		"username": func() (User, error) { return s.UserByUsername(ctx, "alice") },
		"handle":   func() (User, error) { return s.UserByHandle(ctx, u.WebAuthnHandle) },
	} {
		got, err := get()
		if err != nil || got.ID != u.ID || got.Username != "alice" || !bytes.Equal(got.WebAuthnHandle, u.WebAuthnHandle) {
			t.Errorf("user by %s = %+v, %v", name, got, err)
		}
	}
	if _, err := s.User(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("User(999): %v", err)
	}
	// Unverified email does not recover.
	if _, err := s.UserByVerifiedEmail(ctx, "alice@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByVerifiedEmail before verification: %v", err)
	}

	if err := s.SetDisplayName(ctx, u.ID, "Alice A."); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAdmin(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDisplayName(ctx, 999, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetDisplayName(999): %v", err)
	}
	got, _ := s.User(ctx, u.ID)
	if got.DisplayName != "Alice A." || !got.IsAdmin {
		t.Errorf("after updates: %+v", got)
	}
	if err := s.SetEmail(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.User(ctx, u.ID); got.Email != "" || got.EmailVerified() {
		t.Errorf("after SetEmail(\"\"): %+v", got)
	}

	// Disabling signs the user out.
	if _, err := s.CreateSession(ctx, []byte("sess"), u.ID, "ua", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDisabled(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.User(ctx, u.ID); !got.Disabled() || got.DisabledAt != t0 {
		t.Errorf("disabled user = %+v", got)
	}
	if _, _, err := s.SessionUser(ctx, []byte("sess")); !errors.Is(err, ErrNotFound) {
		t.Errorf("session of a disabled user: %v", err)
	}
	if err := s.SetDisabled(ctx, u.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.User(ctx, u.ID); got.Disabled() {
		t.Error("user still disabled")
	}
	if err := s.SetDisabled(ctx, 999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetDisabled(999): %v", err)
	}
	all, err := s.Users(ctx)
	if err != nil || len(all) != 1 {
		t.Errorf("Users = %v, %v", all, err)
	}
}

func TestCredentials(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	other := register(t, s, "bob")

	clk.Advance(time.Minute)
	c2 := newCred("second")
	c2.UserID = u.ID
	c2.Transports = nil
	if err := s.AddCredential(ctx, c2); err != nil {
		t.Fatal(err)
	}
	creds, err := s.Credentials(ctx, u.ID)
	if err != nil || len(creds) != 2 || string(creds[0].ID) != "cred-alice" || string(creds[1].ID) != "second" {
		t.Fatalf("Credentials = %+v, %v", creds, err)
	}
	first := creds[0]
	want := newCred("cred-alice")
	want.UserID, want.CreatedAt = u.ID, t0
	if !credEqual(first, want) {
		t.Errorf("credential round trip:\n got %+v\nwant %+v", first, want)
	}
	if creds[1].Transports != nil {
		t.Errorf("empty transports = %q", creds[1].Transports)
	}

	if err := s.CredentialUsed(ctx, []byte("second"), CredentialUse{SignCount: 7, CloneWarning: true, BackupState: false, UserVerified: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Credential(ctx, []byte("second"))
	if got.SignCount != 7 || !got.CloneWarning || got.BackupState || got.LastUsedAt != t0.Add(time.Minute) {
		t.Errorf("after use: %+v", got)
	}
	if err := s.CredentialUsed(ctx, []byte("nope"), CredentialUse{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("CredentialUsed(nope): %v", err)
	}

	if err := s.RenameCredential(ctx, u.ID, []byte("second"), "Phone"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameCredential(ctx, other.ID, []byte("second"), "Stolen"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename of another user's passkey: %v", err)
	}
	if got, _ := s.Credential(ctx, []byte("second")); got.Name != "Phone" {
		t.Errorf("Name = %q", got.Name)
	}

	if err := s.DeleteCredential(ctx, other.ID, []byte("second")); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete of another user's passkey: %v", err)
	}
	if err := s.DeleteCredential(ctx, u.ID, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCredential(ctx, u.ID, []byte("cred-alice")); !errors.Is(err, ErrLastCredential) {
		t.Errorf("delete of the last passkey: %v", err)
	}
}

func credEqual(a, b Credential) bool {
	return bytes.Equal(a.ID, b.ID) && a.UserID == b.UserID && bytes.Equal(a.PublicKey, b.PublicKey) &&
		a.AttestationType == b.AttestationType && bytes.Equal(a.AAGUID, b.AAGUID) && a.SignCount == b.SignCount &&
		a.CloneWarning == b.CloneWarning && slices.Equal(a.Transports, b.Transports) && a.Attachment == b.Attachment &&
		a.UserPresent == b.UserPresent && a.UserVerified == b.UserVerified && a.BackupEligible == b.BackupEligible &&
		a.BackupState == b.BackupState && a.Name == b.Name && a.CreatedAt.Equal(b.CreatedAt) && a.LastUsedAt.Equal(b.LastUsedAt)
}

func TestDeleteCredentialRace(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	c := newCred("second")
	c.UserID = u.ID
	if err := s.AddCredential(ctx, c); err != nil {
		t.Fatal(err)
	}
	// 2 concurrent deletes of the 2 passkeys: 1 must fail, or the account
	// has none left.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"cred-alice", "second"} {
		wg.Go(func() { errs[i] = s.DeleteCredential(ctx, u.ID, []byte(id)) })
	}
	wg.Wait()
	creds, _ := s.Credentials(ctx, u.ID)
	if len(creds) != 1 {
		t.Fatalf("%d passkeys left, want 1 (errors %v)", len(creds), errs)
	}
	if (errs[0] == nil) == (errs[1] == nil) || !errors.Is(errors.Join(errs...), ErrLastCredential) {
		t.Errorf("errors = %v, want 1 nil and 1 ErrLastCredential", errs)
	}
}

func TestSessions(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	long := strings.Repeat("x", 1000)
	se, err := s.CreateSession(ctx, []byte("a"), u.ID, long, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(se.UserAgent) != maxUserAgent || se.ExpiresAt != t0.Add(30*24*time.Hour) {
		t.Errorf("session = %+v", se)
	}
	if _, err := s.CreateSession(ctx, []byte("a"), u.ID, "", time.Hour); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate session: %v", err)
	}
	if _, err := s.CreateSession(ctx, []byte("b"), u.ID, "phone", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, []byte("c"), u.ID, "tablet", time.Hour); err != nil {
		t.Fatal(err)
	}

	gotSe, gotU, err := s.SessionUser(ctx, []byte("a"))
	if err != nil || gotU.ID != u.ID || gotU.Username != "alice" || gotSe.LastSeenAt != t0 {
		t.Fatalf("SessionUser = %+v, %+v, %v", gotSe, gotU, err)
	}
	if _, _, err := s.SessionUser(ctx, []byte("unknown")); !errors.Is(err, ErrNotFound) {
		t.Errorf("SessionUser(unknown): %v", err)
	}

	// Sliding refresh, at most once per interval.
	tests := []struct {
		advance time.Duration
		want    bool
	}{
		{0, false},
		{30 * time.Minute, false},
		{30 * time.Minute, true}, // 1 h after creation
		{time.Minute, false},
	}
	for i, tt := range tests {
		clk.Advance(tt.advance)
		got, err := s.RefreshSession(ctx, []byte("a"), 30*24*time.Hour, time.Hour)
		if err != nil || got != tt.want {
			t.Errorf("refresh %d: %v, %v; want %v", i, got, err, tt.want)
		}
	}
	gotSe, _, _ = s.SessionUser(ctx, []byte("a"))
	if want := t0.Add(time.Hour + 30*24*time.Hour); gotSe.ExpiresAt != want {
		t.Errorf("ExpiresAt = %v, want %v", gotSe.ExpiresAt, want)
	}

	// "b" and "c" expired after 1 h.
	if _, _, err := s.SessionUser(ctx, []byte("b")); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: %v", err)
	}
	if ok, _ := s.RefreshSession(ctx, []byte("b"), time.Hour, 0); ok {
		t.Error("an expired session was refreshed")
	}
	list, err := s.Sessions(ctx, u.ID)
	if err != nil || len(list) != 1 {
		t.Errorf("Sessions = %v, %v", list, err)
	}
	counts, err := s.Counts(ctx)
	if err != nil || counts.ActiveSessions != 1 || counts.Users != 1 {
		t.Errorf("Counts = %+v, %v", counts, err)
	}
	res, err := s.Cleanup(ctx)
	if err != nil || res.Sessions != 2 {
		t.Errorf("Cleanup = %+v, %v", res, err)
	}

	for _, h := range []string{"d", "e"} {
		if _, err := s.CreateSession(ctx, []byte(h), u.ID, "", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.DeleteUserSessions(ctx, u.ID, []byte("a"))
	if err != nil || n != 2 {
		t.Errorf("DeleteUserSessions(keep a) = %d, %v", n, err)
	}
	if _, _, err := s.SessionUser(ctx, []byte("a")); err != nil {
		t.Errorf("kept session: %v", err)
	}
	if err := s.DeleteSession(ctx, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, []byte("a")); err != nil {
		t.Errorf("second logout: %v", err)
	}
	if n, err := s.DeleteUserSessions(ctx, u.ID, nil); err != nil || n != 0 {
		t.Errorf("DeleteUserSessions(nil) = %d, %v", n, err)
	}
}

func TestCeremonies(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	data := []byte(`{"challenge":"abc"}`)
	for _, c := range []Ceremony{
		{ID: "login1", Kind: CeremonyLogin, Data: data, ExpiresAt: t0.Add(5 * time.Minute)},
		{ID: "add1", Kind: CeremonyAdd, UserID: u.ID, Data: data, ExpiresAt: t0.Add(5 * time.Minute)},
		{ID: "old", Kind: CeremonyLogin, Data: data, ExpiresAt: t0.Add(time.Minute)},
	} {
		if err := s.SaveCeremony(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveCeremony(ctx, Ceremony{ID: "x", Kind: "bogus", Data: data, ExpiresAt: t0}); err == nil {
		t.Error("SaveCeremony accepted an unknown kind")
	}

	if _, err := s.TakeCeremony(ctx, "add1", CeremonyLogin); !errors.Is(err, ErrNotFound) {
		t.Errorf("take with the wrong kind: %v", err)
	}
	c, err := s.TakeCeremony(ctx, "add1", CeremonyAdd)
	if err != nil || c.UserID != u.ID || !bytes.Equal(c.Data, data) || c.ExpiresAt != t0.Add(5*time.Minute) {
		t.Fatalf("TakeCeremony = %+v, %v", c, err)
	}
	if _, err := s.TakeCeremony(ctx, "add1", CeremonyAdd); !errors.Is(err, ErrNotFound) {
		t.Errorf("second take: %v", err)
	}
	c, err = s.TakeCeremony(ctx, "login1", CeremonyLogin)
	if err != nil || c.UserID != 0 {
		t.Errorf("login ceremony = %+v, %v", c, err)
	}
	clk.Advance(time.Minute)
	if _, err := s.TakeCeremony(ctx, "old", CeremonyLogin); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired ceremony: %v", err)
	}
}

func TestEmailTokens(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	bob := register(t, s, "bob")

	// Only the newest link of a purpose works.
	if _, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("v1"), UserID: u.ID, Purpose: PurposeVerify, Email: "alice@example.com"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	tok, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("v2"), UserID: u.ID, Purpose: PurposeVerify, Email: " alice@example.com "}, time.Hour)
	if err != nil || tok.Email != "alice@example.com" || tok.ExpiresAt != t0.Add(time.Hour) {
		t.Fatalf("CreateEmailToken = %+v, %v", tok, err)
	}
	if _, err := s.ValidEmailToken(ctx, []byte("v1"), PurposeVerify); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("replaced token: %v", err)
	}
	if _, err := s.ValidEmailToken(ctx, []byte("v2"), PurposeRecover); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("token with the wrong purpose: %v", err)
	}
	if p, err := s.PendingEmail(ctx, u.ID); err != nil || p.Email != "alice@example.com" {
		t.Errorf("PendingEmail = %+v, %v", p, err)
	}

	verified, err := s.VerifyEmail(ctx, []byte("v2"))
	if err != nil || !verified.EmailVerified() || verified.EmailVerifiedAt != t0 {
		t.Fatalf("VerifyEmail = %+v, %v", verified, err)
	}
	if _, err := s.VerifyEmail(ctx, []byte("v2")); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("second VerifyEmail: %v", err)
	}
	if _, err := s.PendingEmail(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("PendingEmail after verification: %v", err)
	}
	if got, err := s.UserByVerifiedEmail(ctx, " ALICE@example.com"); err != nil || got.ID != u.ID {
		t.Errorf("UserByVerifiedEmail = %+v, %v", got, err)
	}

	// Bob tries to take Alice's verified address: the token stays unused.
	if _, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("b1"), UserID: bob.ID, Purpose: PurposeVerify, Email: "Alice@Example.com"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyEmail(ctx, []byte("b1")); !errors.Is(err, ErrConflict) {
		t.Errorf("verify a taken address: %v", err)
	}
	if _, err := s.ValidEmailToken(ctx, []byte("b1"), PurposeVerify); err != nil {
		t.Errorf("token after a conflict: %v", err)
	}

	// Change of address: the old one works until the new one is verified.
	if _, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("v3"), UserID: u.ID, Purpose: PurposeVerify, Email: "new@example.com"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.User(ctx, u.ID); got.Email != "alice@example.com" || !got.EmailVerified() {
		t.Errorf("address changed before verification: %+v", got)
	}
	clk.Advance(time.Hour)
	if _, err := s.VerifyEmail(ctx, []byte("v3")); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("expired verify token: %v", err)
	}

	// Use once.
	if _, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("r1"), UserID: u.ID, Purpose: PurposeRecover, Email: "alice@example.com"}, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, err := s.UseEmailToken(ctx, []byte("r1"), PurposeRecover); err != nil || got.UserID != u.ID || got.UsedAt.IsZero() {
		t.Errorf("UseEmailToken = %+v, %v", got, err)
	}
	if _, err := s.UseEmailToken(ctx, []byte("r1"), PurposeRecover); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("second UseEmailToken: %v", err)
	}
}

func TestRecoverAccount(t *testing.T) {
	for _, removeOthers := range []bool{false, true} {
		t.Run(fmt.Sprint("removeOthers=", removeOthers), func(t *testing.T) {
			s, _ := newStore(t)
			ctx := context.Background()
			u := register(t, s, "alice")
			extra := newCred("extra")
			extra.UserID = u.ID
			if err := s.AddCredential(ctx, extra); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateSession(ctx, []byte("old-session"), u.ID, "", time.Hour); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateEmailToken(ctx, EmailToken{TokenHash: []byte("r"), UserID: u.ID, Purpose: PurposeRecover, Email: u.Email}, 15*time.Minute); err != nil {
				t.Fatal(err)
			}
			got, removed, err := s.RecoverAccount(ctx, []byte("r"), newCred("recovered"), removeOthers)
			if err != nil || got.ID != u.ID {
				t.Fatalf("RecoverAccount = %+v, %v", got, err)
			}
			creds, _ := s.Credentials(ctx, u.ID)
			_, _, sessErr := s.SessionUser(ctx, []byte("old-session"))
			if removeOthers {
				if removed != 2 || len(creds) != 1 || string(creds[0].ID) != "recovered" || !errors.Is(sessErr, ErrNotFound) {
					t.Errorf("removed %d, left %d passkeys, session %v", removed, len(creds), sessErr)
				}
			} else if removed != 0 || len(creds) != 3 || sessErr != nil {
				t.Errorf("removed %d, left %d passkeys, session %v", removed, len(creds), sessErr)
			}
			if _, _, err := s.RecoverAccount(ctx, []byte("r"), newCred("again"), false); !errors.Is(err, ErrTokenInvalid) {
				t.Errorf("second recovery with the token: %v", err)
			}
			if _, err := s.Credential(ctx, []byte("again")); !errors.Is(err, ErrNotFound) {
				t.Errorf("passkey of a refused recovery: %v", err)
			}
		})
	}
}

func TestCode(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	if _, err := s.SaveCode(ctx, u.ID, "07-retry", "retry.go", "v1"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Second)
	at, err := s.SaveCode(ctx, u.ID, "07-retry", "retry.go", "v2")
	if err != nil || at != t0.Add(time.Second) {
		t.Fatalf("SaveCode = %v, %v", at, err)
	}
	if _, err := s.SaveCode(ctx, u.ID, "07-retry", "retryafter.go", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCode(ctx, u.ID, "01-logfmt", "logfmt.go", "other"); err != nil {
		t.Fatal(err)
	}
	files, err := s.Code(ctx, u.ID, "07-retry")
	if err != nil || len(files) != 2 || files["retry.go"].Content != "v2" || files["retry.go"].UpdatedAt != at {
		t.Fatalf("Code = %+v, %v", files, err)
	}
	if err := s.DeleteCode(ctx, u.ID, "07-retry", "retry.go"); err != nil {
		t.Fatal(err)
	}
	if files, _ := s.Code(ctx, u.ID, "07-retry"); len(files) != 1 {
		t.Errorf("after DeleteCode: %+v", files)
	}
	if _, err := s.SaveCode(ctx, 999, "07-retry", "retry.go", "x"); err == nil {
		t.Error("SaveCode for an unknown user succeeded (foreign keys off?)")
	}
}

func TestProgress(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	if _, err := s.Progress(ctx, u.ID, "07-retry"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Progress before start: %v", err)
	}
	p, err := s.StartExercise(ctx, u.ID, "07-retry")
	if err != nil || p.Status != StatusStarted || p.StartedAt != t0 {
		t.Fatalf("StartExercise = %+v, %v", p, err)
	}
	clk.Advance(time.Minute)
	if p, _ := s.StartExercise(ctx, u.ID, "07-retry"); p.StartedAt != t0 {
		t.Errorf("second StartExercise moved StartedAt to %v", p.StartedAt)
	}

	for range 3 {
		if err := s.AddActiveTime(ctx, u.ID, "07-retry", 15*time.Second+500*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddActiveTime(ctx, u.ID, "07-retry", -time.Second); err == nil {
		t.Error("negative active time accepted")
	}
	result := json.RawMessage(`{"passed":3,"failed":1}`)
	if err := s.RecordRun(ctx, u.ID, "07-retry", result); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRun(ctx, u.ID, "07-retry", json.RawMessage(`{bad`)); err == nil {
		t.Error("RecordRun accepted bad JSON")
	}
	if err := s.SetNotes(ctx, u.ID, "07-retry", "jitter is [0, backoff)"); err != nil {
		t.Fatal(err)
	}
	// Any update starts an exercise.
	if err := s.RecordRun(ctx, u.ID, "08-ratelimit", nil); err != nil {
		t.Fatal(err)
	}

	p, err = s.Progress(ctx, u.ID, "07-retry")
	if err != nil || p.ActiveSeconds != 45 || p.Runs != 1 || p.LastRunAt != t0.Add(time.Minute) ||
		string(p.LastResult) != string(result) || p.Notes != "jitter is [0, backoff)" || p.Passed() {
		t.Fatalf("Progress = %+v, %v", p, err)
	}

	clk.Advance(time.Hour)
	first, err := s.MarkPassed(ctx, u.ID, "07-retry")
	if err != nil || !first {
		t.Fatalf("MarkPassed = %v, %v", first, err)
	}
	clk.Advance(time.Hour)
	if first, _ := s.MarkPassed(ctx, u.ID, "07-retry"); first {
		t.Error("second MarkPassed reported first")
	}
	p, _ = s.Progress(ctx, u.ID, "07-retry")
	if !p.Passed() || p.PassedAt != t0.Add(time.Minute+time.Hour) {
		t.Errorf("passed progress = %+v", p)
	}
	all, err := s.UserProgress(ctx, u.ID)
	if err != nil || len(all) != 2 || all[0].Exercise != "07-retry" || all[1].Runs != 1 {
		t.Errorf("UserProgress = %+v, %v", all, err)
	}
	passed, err := s.PassedExercises(ctx, u.ID)
	if err != nil || !slices.Equal(passed, []string{"07-retry"}) {
		t.Errorf("PassedExercises = %v, %v", passed, err)
	}
}

func TestMarkPassedRace(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	var wg sync.WaitGroup
	var firsts atomic.Int32
	for range 8 {
		wg.Go(func() {
			first, err := s.MarkPassed(ctx, u.ID, "01-logfmt")
			if err != nil {
				t.Error(err)
			}
			if first {
				firsts.Add(1)
			}
		})
	}
	wg.Wait()
	if firsts.Load() != 1 {
		t.Errorf("%d calls reported the first pass, want 1", firsts.Load())
	}
}

func TestReveals(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	steps := []struct {
		level   int
		created bool
		err     error
	}{
		{2, false, ErrRevealOrder},
		{1, true, nil},
		{1, false, nil}, // again: idempotent
		{3, false, ErrRevealOrder},
		{2, true, nil},
		{3, true, nil},
		{4, true, nil},
		{5, true, nil},
		{6, true, nil},
	}
	for i, st := range steps {
		clk.Advance(time.Minute)
		r, created, err := s.RecordReveal(ctx, u.ID, "07-retry", st.level)
		if !errors.Is(err, st.err) || created != st.created {
			t.Errorf("step %d: reveal %d = %v, %v; want %v, %v", i, st.level, created, err, st.created, st.err)
			continue
		}
		if err == nil && (r.Level != st.level || r.RevealedAt.IsZero()) {
			t.Errorf("step %d: reveal = %+v", i, r)
		}
	}
	for _, bad := range []int{0, 7} {
		if _, _, err := s.RecordReveal(ctx, u.ID, "07-retry", bad); err == nil {
			t.Errorf("level %d accepted", bad)
		}
	}
	rs, err := s.Reveals(ctx, u.ID, "07-retry")
	if err != nil || len(rs) != 6 {
		t.Fatalf("Reveals = %v, %v", rs, err)
	}
	if rs[0].RevealedAt != t0.Add(2*time.Minute) {
		t.Errorf("level 1 revealed at %v, want the first reveal", rs[0].RevealedAt)
	}
	if _, _, err := s.RecordReveal(ctx, u.ID, "01-logfmt", 1); err != nil {
		t.Fatal(err)
	}
	all, err := s.UserReveals(ctx, u.ID)
	if err != nil || len(all) != 7 || all[0].Exercise != "01-logfmt" {
		t.Errorf("UserReveals = %v, %v", all, err)
	}
}

func TestRevealRace(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	// Level 1 and 2 at once: level 2 succeeds only if level 1 committed
	// first, never without it.
	for round := range 20 {
		ex := fmt.Sprintf("ex-%d", round)
		var wg sync.WaitGroup
		var err2 error
		wg.Go(func() {
			if _, _, err := s.RecordReveal(ctx, u.ID, ex, 1); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() { _, _, err2 = s.RecordReveal(ctx, u.ID, ex, 2) })
		wg.Wait()
		rs, _ := s.Reveals(ctx, u.ID, ex)
		if err2 == nil && len(rs) != 2 || err2 != nil && (len(rs) != 1 || !errors.Is(err2, ErrRevealOrder)) {
			t.Fatalf("round %d: level 2 err %v, reveals %v", round, err2, rs)
		}
	}
}

func TestCertificates(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")

	c1 := Certificate{ID: "GP-AAAA-BBBB-CCCC", UserID: u.ID, Kind: "tier1", NameOnCert: "Alice", IssuedAt: t0,
		Payload: []byte(`{"id":"GP-AAAA-BBBB-CCCC"}`), Signature: []byte("sig1")}
	old, err := s.IssueCertificate(ctx, c1)
	if err != nil || old != "" {
		t.Fatalf("IssueCertificate = %q, %v", old, err)
	}
	clk.Advance(time.Hour)
	c2 := Certificate{ID: "GP-DDDD-EEEE-FFFF", UserID: u.ID, Kind: "tier1", NameOnCert: "Alice A.", IssuedAt: t0.Add(time.Hour),
		ShowStats: true, Stats: json.RawMessage(`{"runs":3}`), Payload: []byte(`{"id":"GP-DDDD-EEEE-FFFF"}`), Signature: []byte("sig2")}
	old, err = s.IssueCertificate(ctx, c2)
	if err != nil || old != c1.ID {
		t.Fatalf("re-issue = %q, %v", old, err)
	}
	got1, err := s.Certificate(ctx, "gp-aaaa-bbbb-cccc ")
	if err != nil || !got1.Revoked() || got1.ReplacedBy != c2.ID || got1.RevokedAt != t0.Add(time.Hour) {
		t.Errorf("old certificate = %+v, %v", got1, err)
	}
	got2, err := s.Certificate(ctx, c2.ID)
	if err != nil || got2.Revoked() || !got2.ShowStats || string(got2.Stats) != `{"runs":3}` ||
		string(got2.Payload) != string(c2.Payload) || string(got2.Signature) != "sig2" || got2.IssuedAt != c2.IssuedAt {
		t.Errorf("new certificate = %+v, %v", got2, err)
	}

	// An ID collision is a conflict, and changes nothing.
	dup := c2
	dup.Kind = "tier2"
	if _, err := s.IssueCertificate(ctx, dup); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate ID: %v", err)
	}
	if _, err := s.IssueCertificate(ctx, Certificate{ID: "GP-X", UserID: u.ID, Kind: "tier11", IssuedAt: t0, Payload: []byte("{}"), Signature: []byte("s")}); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := s.IssueCertificate(ctx, Certificate{ID: "GP-Y", UserID: u.ID, Kind: "tier2", IssuedAt: t0, Payload: []byte("{"), Signature: []byte("s")}); err == nil {
		t.Error("bad payload accepted")
	}

	course := Certificate{ID: "GP-GGGG-HHHH-JJJJ", UserID: u.ID, Kind: "course", NameOnCert: "Alice", IssuedAt: t0,
		Payload: []byte(`{}`), Signature: []byte("s")}
	if _, err := s.IssueCertificate(ctx, course); err != nil {
		t.Fatal(err)
	}
	list, err := s.UserCertificates(ctx, u.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("UserCertificates = %v, %v", list, err)
	}
	if err := s.RevokeCertificate(ctx, course.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeCertificate(ctx, course.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second revoke: %v", err)
	}
	if got, _ := s.Certificate(ctx, course.ID); !got.Revoked() || got.ReplacedBy != "" {
		t.Errorf("revoked certificate = %+v", got)
	}
	if _, err := s.Certificate(ctx, "GP-NONE"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown certificate: %v", err)
	}
	counts, _ := s.Counts(ctx)
	if counts.Certificates != 1 {
		t.Errorf("Counts.Certificates = %d, want 1", counts.Certificates)
	}
}

func TestSettings(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.Setting(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Setting before set: %v", err)
	}
	// Concurrent first starts agree on 1 value.
	var wg sync.WaitGroup
	got := make([]string, 8)
	for i := range got {
		wg.Go(func() {
			v, err := s.SetSettingIfAbsent(ctx, "key", fmt.Sprint("value-", i))
			if err != nil {
				t.Error(err)
			}
			got[i] = v
		})
	}
	wg.Wait()
	for _, v := range got {
		if v != got[0] {
			t.Fatalf("SetSettingIfAbsent values differ: %q", got)
		}
	}
	if err := s.SetSetting(ctx, "key", "replaced"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Setting(ctx, "key"); err != nil || v != "replaced" {
		t.Errorf("Setting = %q, %v", v, err)
	}
	if v, _ := s.SetSettingIfAbsent(ctx, "key", "ignored"); v != "replaced" {
		t.Errorf("SetSettingIfAbsent over a value = %q", v)
	}
}

func TestAudit(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	for i := range 5 {
		clk.Advance(time.Second)
		user := u.ID
		if i == 0 {
			user = 0
		}
		if err := s.Audit(ctx, user, "login", map[string]int{"n": i}, "10.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Audit(ctx, u.ID, "bad", func() {}, ""); err == nil {
		t.Error("Audit accepted a value JSON cannot encode")
	}
	if err := s.Audit(ctx, u.ID, "nil detail", nil, ""); err != nil {
		t.Fatal(err)
	}
	page, err := s.AuditLog(ctx, AuditQuery{Limit: 3})
	if err != nil || len(page) != 3 || page[0].Event != "nil detail" || string(page[0].Detail) != "{}" {
		t.Fatalf("AuditLog = %+v, %v", page, err)
	}
	rest, err := s.AuditLog(ctx, AuditQuery{BeforeID: page[2].ID})
	if err != nil || len(rest) != 3 || rest[2].UserID != 0 || string(rest[2].Detail) != `{"n":0}` || rest[2].IP != "10.0.0.1" {
		t.Errorf("second page = %+v, %v", rest, err)
	}
	mine, err := s.AuditLog(ctx, AuditQuery{UserID: u.ID})
	if err != nil || len(mine) != 5 {
		t.Errorf("AuditLog for the user = %d entries, %v", len(mine), err)
	}
}

func TestUserSummaries(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	a := register(t, s, "alice")
	register(t, s, "bob")
	if err := s.AddActiveTime(ctx, a.ID, "01-logfmt", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRun(ctx, a.ID, "02-accesslog", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkPassed(ctx, a.ID, "01-logfmt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RecordReveal(ctx, a.ID, "01-logfmt", 1); err != nil {
		t.Fatal(err)
	}
	sums, err := s.UserSummaries(ctx)
	if err != nil || len(sums) != 2 {
		t.Fatalf("UserSummaries = %v, %v", sums, err)
	}
	if got := sums[0]; got.Username != "alice" || got.Started != 2 || got.Passed != 1 || got.ActiveSeconds != 60 ||
		got.Runs != 1 || got.Reveals != 1 || got.LastActive != t0 {
		t.Errorf("alice = %+v", got)
	}
	if got := sums[1]; got.Username != "bob" || got.Started != 0 || !got.LastActive.IsZero() {
		t.Errorf("bob = %+v", got)
	}
	counts, _ := s.Counts(ctx)
	if counts.Passed != 1 || counts.Users != 2 {
		t.Errorf("Counts = %+v", counts)
	}
}

func TestProbe(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	for range 2 {
		if err := s.Probe(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// A database that cannot be written fails the probe, although reads
	// still work.
	ro, err := sql.Open("sqlite", dsn(s.Path(), true))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	roStore := &Store{db: ro, path: s.Path(), now: s.now}
	if _, err := roStore.CountUsers(ctx); err != nil {
		t.Fatalf("read on the read-only store: %v", err)
	}
	if err := roStore.Probe(ctx); err == nil {
		t.Error("Probe on a read-only database succeeded")
	}
}

func TestBackup(t *testing.T) {
	s, clk := newStore(t)
	ctx := context.Background()
	u := register(t, s, "alice")
	dir := filepath.Join(t.TempDir(), "backups")

	var paths []string
	for day := range 8 {
		if _, err := s.SaveCode(ctx, u.ID, "01-logfmt", "logfmt.go", fmt.Sprint("day ", day)); err != nil {
			t.Fatal(err)
		}
		p, err := s.Backup(ctx, dir)
		if err != nil {
			t.Fatalf("day %d: %v", day, err)
		}
		paths = append(paths, p)
		clk.Advance(24 * time.Hour)
	}
	if filepath.Base(paths[0]) != "go-practice-monday.db" || paths[7] != paths[0] {
		t.Errorf("backup paths %q", paths)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 7 {
		t.Fatalf("backup dir has %d entries, want 7 (%v)", len(entries), err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover %s", e.Name())
		}
	}

	// The Monday file holds day 7's data and opens as a store.
	b, err := Open(ctx, paths[7])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	files, err := b.Code(ctx, u.ID, "01-logfmt")
	if err != nil || files["logfmt.go"].Content != "day 7" {
		t.Errorf("backup content = %+v, %v", files, err)
	}

	// A leftover temporary file from a crash does not block the backup.
	if err := os.WriteFile(filepath.Join(dir, s.BackupName()+".tmp"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Backup(ctx, dir); err != nil {
		t.Errorf("backup over a leftover: %v", err)
	}
}

func TestCheckIntegrity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, bytes.Repeat([]byte("not a database "), 512), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkIntegrity(ctx, junk); err == nil {
		t.Error("checkIntegrity accepted junk")
	}
	empty := filepath.Join(dir, "empty.db")
	if err := checkIntegrity(ctx, empty); err == nil {
		t.Error("checkIntegrity accepted an empty database")
	}
}

func TestTimeFormat(t *testing.T) {
	// Fixed width, so text order is time order.
	times := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 0, 0, 0, 5e8, time.UTC),
		time.Date(2026, 1, 1, 1, 0, 0, 75e7, time.FixedZone("x", 3600)),
		time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
	}
	var texts []string
	for _, tm := range times {
		texts = append(texts, fmtTime(tm))
	}
	if !slices.IsSorted(texts) || len(texts[0]) != len(texts[1]) {
		t.Errorf("formatted times %q are not sortable", texts)
	}
	for i, tm := range times {
		got, err := parseTime(texts[i])
		if err != nil || !got.Equal(tm) {
			t.Errorf("parseTime(%q) = %v, %v; want %v", texts[i], got, err, tm)
		}
	}
	var dst time.Time
	if err := tcol(&dst).Scan(42); err == nil {
		t.Error("timeCol scanned an int")
	}
}
