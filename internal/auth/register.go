package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

// handleBytes is the size of a WebAuthn user handle. It is random, so it
// tells an authenticator nothing about the account.
const handleBytes = 32

// EnsureSetupLink returns a fresh setup link while the site has no
// account, and "" once it has one. Every start replaces the unused setup
// token, so a pod that restarts before setup logs a link that works; the
// old one stops working.
func (s *Service) EnsureSetupLink(ctx context.Context) (string, error) {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: setup: %w", err)
	}
	if n > 0 {
		return "", nil
	}
	token, hash := newToken()
	if err := s.store.ReplaceSetupToken(ctx, hash); err != nil {
		return "", fmt.Errorf("auth: setup: %w", err)
	}
	return s.link("/setup", url.Values{"token": {token}}), nil
}

// CheckSetupToken reports whether a setup link still works, so the setup
// page can say so before the user fills in the form.
func (s *Service) CheckSetupToken(ctx context.Context, token string) error {
	hash, ok := hashToken(token)
	if !ok {
		return errBadToken
	}
	return storeError(s.store.ValidSetupToken(ctx, hash), "check setup token")
}

// CheckInvite returns a usable invite.
func (s *Service) CheckInvite(ctx context.Context, token string) (store.Invite, error) {
	hash, ok := hashToken(token)
	if !ok {
		return store.Invite{}, errBadToken
	}
	inv, err := s.store.ValidInvite(ctx, hash)
	if err != nil {
		return store.Invite{}, storeError(err, "check invite")
	}
	return inv, nil
}

// SignUp is the form of the sign-up page. Exactly 1 of Invite and Setup is
// set: the token from the link.
type SignUp struct {
	Invite      string `json:"invite"`
	Setup       string `json:"setup"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// BeginRegistration checks the sign-up form and its token and starts the
// passkey ceremony. The account is created only by FinishRegistration,
// which checks the token again in the same transaction.
func (s *Service) BeginRegistration(ctx context.Context, in SignUp, c Client) (Begin, error) {
	if err := allow(s.beginLimit, c.RateKey()); err != nil {
		return Begin{}, err
	}
	var d ceremonyData
	switch {
	case in.Setup != "" && in.Invite == "":
		hash, ok := hashToken(in.Setup)
		if !ok {
			return Begin{}, errBadToken
		}
		if err := s.store.ValidSetupToken(ctx, hash); err != nil {
			return Begin{}, storeError(err, "check setup token")
		}
		d.TokenHash, d.Setup = hash, true
	case in.Invite != "" && in.Setup == "":
		hash, ok := hashToken(in.Invite)
		if !ok {
			return Begin{}, errBadToken
		}
		if _, err := s.store.ValidInvite(ctx, hash); err != nil {
			return Begin{}, storeError(err, "check invite")
		}
		d.TokenHash = hash
	default:
		return Begin{}, badInput("no_invite", "Sign up needs an invite link.")
	}

	username, err := cleanUsername(in.Username)
	if err != nil {
		return Begin{}, err
	}
	display, err := cleanDisplayName(in.DisplayName)
	if err != nil {
		return Begin{}, err
	}
	email, err := cleanEmail(in.Email)
	if err != nil {
		return Begin{}, err
	}
	// The unique index decides in the end; this check only saves the user
	// a passkey prompt for a name that is taken.
	if _, err := s.store.UserByUsername(ctx, username); err == nil {
		return Begin{}, storeError(&store.ConflictError{Column: "users.username"}, "")
	} else if !errors.Is(err, store.ErrNotFound) {
		return Begin{}, fmt.Errorf("auth: begin registration: %w", err)
	}

	handle := make([]byte, handleBytes)
	_, _ = rand.Read(handle) // never fails on supported platforms
	d.User = &pendingUser{Handle: handle, Username: username, DisplayName: display, Email: email}
	user := &waUser{handle: handle, name: username, displayName: display}
	creation, session, err := s.wa.BeginRegistration(user, registrationOptions(nil)...)
	if err != nil {
		return Begin{}, fmt.Errorf("auth: begin registration: %w", err)
	}
	d.Session = *session
	id, err := s.saveCeremony(ctx, store.CeremonyRegister, 0, d)
	if err != nil {
		return Begin{}, err
	}
	return Begin{Ceremony: id, Options: creation}, nil
}

// FinishRegistration verifies the new passkey and creates the account,
// using the invite or setup token in the same transaction. The caller
// starts the session.
func (s *Service) FinishRegistration(ctx context.Context, in Finish, c Client) (store.User, error) {
	_, d, err := s.takeCeremony(ctx, in.Ceremony, store.CeremonyRegister)
	if err != nil {
		return store.User{}, err
	}
	if d.User == nil {
		return store.User{}, errCeremony
	}
	p := d.User
	cred, err := s.createCredential(&waUser{handle: p.Handle, name: p.Username, displayName: p.DisplayName}, d.Session, in.Credential)
	if err != nil {
		return store.User{}, err
	}
	reg := store.Registration{
		User:       store.NewUser{Username: p.Username, DisplayName: p.DisplayName, Email: p.Email, WebAuthnHandle: p.Handle},
		Credential: toStore(cred, 0, deviceName(c.UserAgent)),
	}
	var u store.User
	if d.Setup {
		u, err = s.store.RegisterWithSetupToken(ctx, d.TokenHash, reg)
	} else {
		u, err = s.store.RegisterWithInvite(ctx, d.TokenHash, reg)
	}
	if err != nil {
		return store.User{}, storeError(err, "register")
	}
	s.audit(ctx, u.ID, "user.registered", map[string]any{"setup": d.Setup, "admin": u.IsAdmin}, c)
	if u.Email != "" {
		if err := s.sendVerification(ctx, u, u.Email); err != nil {
			// The account exists; the user can ask for a new link on the
			// account page.
			s.log.Error("verification email after sign-up", "user_id", u.ID, "err", err)
		}
	}
	return u, nil
}

// sendVerification stores a verify token for address and mails the link.
func (s *Service) sendVerification(ctx context.Context, u store.User, address string) error {
	token, hash := newToken()
	if _, err := s.store.CreateEmailToken(ctx, store.EmailToken{
		TokenHash: hash, UserID: u.ID, Purpose: store.PurposeVerify, Email: address,
	}, VerifyTTL); err != nil {
		return fmt.Errorf("auth: verify token: %w", err)
	}
	s.notify(address, mail.VerifyEmail{
		Name:     u.DisplayName,
		Link:     s.link("/verify-email", url.Values{"token": {token}}),
		ValidFor: VerifyTTL,
	}, "verify")
	return nil
}

// errDisabled is the answer for a disabled account.
var errDisabled = newError(http.StatusForbidden, "account_disabled",
	"This account is disabled. Ask the person who invited you.", nil)
