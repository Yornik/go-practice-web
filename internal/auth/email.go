package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

// RequestRecovery mails a recovery link when a user with that verified
// address exists. The answer is the same either way, and the lookup and
// the mail happen in the background, so neither the reply nor its timing
// tells whether an address has an account.
//
// The context is not used: the work runs detached, so a client that hangs
// up cannot tell from the timing that it stopped early.
func (s *Service) RequestRecovery(_ context.Context, email string, c Client) error {
	addr, err := cleanEmail(email)
	if err != nil || addr == "" {
		return badInput("bad_email", "Enter an email address like name@example.com.")
	}
	if err := allow(s.recoverIP, c.RateKey()); err != nil {
		return err
	}
	if err := allow(s.recoverEmail, strings.ToLower(addr)); err != nil {
		return err
	}
	s.bg.Go(func() {
		bctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
		defer cancel()
		if err := s.sendRecovery(bctx, addr, c); err != nil {
			s.log.Error("recovery request", "err", err)
		}
	})
	return nil
}

func (s *Service) sendRecovery(ctx context.Context, addr string, c Client) error {
	u, err := s.store.UserByVerifiedEmail(ctx, addr)
	if errors.Is(err, store.ErrNotFound) {
		s.audit(ctx, 0, "recovery.unknown_email", nil, c)
		return nil
	}
	if err != nil {
		return err
	}
	if u.Disabled() {
		s.audit(ctx, u.ID, "recovery.disabled", nil, c)
		return nil
	}
	token, hash := newToken()
	if _, err := s.store.CreateEmailToken(ctx, store.EmailToken{
		TokenHash: hash, UserID: u.ID, Purpose: store.PurposeRecover, Email: u.Email,
	}, RecoveryTTL); err != nil {
		return err
	}
	s.audit(ctx, u.ID, "recovery.requested", nil, c)
	return s.mailer.Send(ctx, u.Email, mail.Recovery{
		Name:     u.DisplayName,
		Link:     s.link("/recover/confirm", url.Values{"token": {token}}),
		ValidFor: RecoveryTTL,
	})
}

// RecoveryUser returns the account a recovery link belongs to, so the
// confirm page can greet the user and say when a link is dead.
func (s *Service) RecoveryUser(ctx context.Context, token string) (store.User, error) {
	hash, ok := hashToken(token)
	if !ok {
		return store.User{}, errBadToken
	}
	t, err := s.store.ValidEmailToken(ctx, hash, store.PurposeRecover)
	if err != nil {
		return store.User{}, storeError(err, "recovery token")
	}
	u, err := s.store.User(ctx, t.UserID)
	if err != nil {
		return store.User{}, storeError(err, "recovery user")
	}
	if u.Disabled() {
		return store.User{}, errDisabled
	}
	return u, nil
}

// BeginRecovery starts the ceremony that adds a passkey through a recovery
// link. Old passkeys are not excluded: the user may still hold 1 on this
// device, and re-creating it is a valid way out.
func (s *Service) BeginRecovery(ctx context.Context, token string, c Client) (Begin, error) {
	if err := allow(s.beginLimit, c.RateKey()); err != nil {
		return Begin{}, err
	}
	u, err := s.RecoveryUser(ctx, token)
	if err != nil {
		return Begin{}, err
	}
	hash, _ := hashToken(token) // RecoveryUser checked it
	wu := &waUser{handle: u.WebAuthnHandle, name: u.Username, displayName: u.DisplayName}
	creation, session, err := s.wa.BeginRegistration(wu, registrationOptions(nil)...)
	if err != nil {
		return Begin{}, fmt.Errorf("auth: begin recovery: %w", err)
	}
	id, err := s.saveCeremony(ctx, store.CeremonyRecover, u.ID, ceremonyData{Session: *session, TokenHash: hash})
	if err != nil {
		return Begin{}, err
	}
	return Begin{Ceremony: id, Options: creation}, nil
}

// Recover is the finish request of BeginRecovery.
type Recover struct {
	Finish
	// RemoveOthers deletes every other passkey and session: the way out
	// when a device was stolen.
	RemoveOthers bool `json:"removeOthers"`
}

// FinishRecovery verifies the new passkey and adds it, using the recovery
// token in the same transaction. The caller starts the session.
func (s *Service) FinishRecovery(ctx context.Context, in Recover, c Client) (store.User, error) {
	cer, d, err := s.takeCeremony(ctx, in.Ceremony, store.CeremonyRecover)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.store.User(ctx, cer.UserID)
	if err != nil {
		return store.User{}, storeError(err, "recovery user")
	}
	if u.Disabled() {
		return store.User{}, errDisabled
	}
	wu := &waUser{handle: u.WebAuthnHandle, name: u.Username, displayName: u.DisplayName}
	cred, err := s.createCredential(wu, d.Session, in.Credential)
	if err != nil {
		return store.User{}, err
	}
	name := deviceName(c.UserAgent)
	u, removed, err := s.store.RecoverAccount(ctx, d.TokenHash, toStore(cred, u.ID, name), in.RemoveOthers)
	if err != nil {
		return store.User{}, storeError(err, "recover account")
	}
	s.audit(ctx, u.ID, "recovery.used", map[string]any{"passkey": name, "removed": removed}, c)
	s.notify(u.Email, mail.RecoveryUsed{Name: u.DisplayName, Passkey: name, When: s.now(), RemovedOthers: int(removed)}, "recovery-used")
	return u, nil
}

// VerifyEmail uses a verify link. When it replaces another verified
// address, the new address is told so.
func (s *Service) VerifyEmail(ctx context.Context, token string, c Client) (store.User, error) {
	hash, ok := hashToken(token)
	if !ok {
		return store.User{}, errBadToken
	}
	t, err := s.store.ValidEmailToken(ctx, hash, store.PurposeVerify)
	if err != nil {
		return store.User{}, storeError(err, "verify token")
	}
	before, err := s.store.User(ctx, t.UserID)
	if err != nil {
		return store.User{}, storeError(err, "verify user")
	}
	u, err := s.store.VerifyEmail(ctx, hash)
	if err != nil {
		return store.User{}, storeError(err, "verify email")
	}
	s.audit(ctx, u.ID, "email.verified", map[string]string{"email": u.Email}, c)
	if before.EmailVerified() && !strings.EqualFold(before.Email, u.Email) {
		s.notify(u.Email, mail.EmailChangedNew{EmailChange: mail.EmailChange{
			Name: u.DisplayName, Old: before.Email, New: u.Email, When: s.now(),
		}}, "email-changed-new")
	}
	return u, nil
}

// Account is the form of the account page.
type Account struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// AccountResult tells the page what happened to the email address.
type AccountResult struct {
	User store.User `json:"-"`
	// VerificationSent is true when a verify link went to Email.
	VerificationSent bool   `json:"verificationSent"`
	Email            string `json:"email,omitempty"`
}

// UpdateAccount changes the display name and starts an email change. A
// new address takes effect when its link is used; until then a verified
// old address keeps working for recovery and is told about the change.
func (s *Service) UpdateAccount(ctx context.Context, u store.User, in Account, c Client) (AccountResult, error) {
	display, err := cleanDisplayName(in.DisplayName)
	if err != nil {
		return AccountResult{}, err
	}
	// An address can change but not go: it is the only way back in.
	email, err := requiredEmail(in.Email)
	if err != nil {
		return AccountResult{}, err
	}
	// A change of address mails a link to it from the site's domain. The
	// limits come before any change, so a refused request changes nothing.
	if !strings.EqualFold(email, u.Email) || !u.EmailVerified() {
		if err := s.allowVerification(u, email); err != nil {
			return AccountResult{}, err
		}
	}
	if display != u.DisplayName {
		if err := s.store.SetDisplayName(ctx, u.ID, display); err != nil {
			return AccountResult{}, storeError(err, "set display name")
		}
		u.DisplayName = display
		s.audit(ctx, u.ID, "account.display_name", nil, c)
	}
	res := AccountResult{User: u}
	switch {
	case strings.EqualFold(email, u.Email) && u.EmailVerified():
		// No change.
	default:
		if other, err := s.store.UserByVerifiedEmail(ctx, email); err == nil && other.ID != u.ID {
			return AccountResult{}, storeError(&store.ConflictError{Column: "users.email"}, "")
		}
		if !u.EmailVerified() && !strings.EqualFold(email, u.Email) {
			// An unverified address protects nothing, so it is replaced at
			// once; the unique index catches an address another account
			// holds.
			if err := s.store.SetEmail(ctx, u.ID, email); err != nil {
				return AccountResult{}, storeError(err, "set email")
			}
			res.User.Email = email
		}
		if err := s.sendVerification(ctx, res.User, email); err != nil {
			return AccountResult{}, err
		}
		res.VerificationSent, res.Email = true, email
		s.audit(ctx, u.ID, "account.email_change", map[string]string{"email": email}, c)
		if u.EmailVerified() && !strings.EqualFold(email, u.Email) {
			s.notify(u.Email, mail.EmailChangedOld{EmailChange: mail.EmailChange{
				Name: u.DisplayName, Old: u.Email, New: email, When: s.now(), Pending: true,
			}}, "email-changed-old")
		}
	}
	return res, nil
}

// PendingEmail returns the address waiting for verification, or "".
func (s *Service) PendingEmail(ctx context.Context, u store.User) (string, error) {
	t, err := s.store.PendingEmail(ctx, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("auth: pending email: %w", err)
	}
	return t.Email, nil
}

// allowVerification applies the limits on verification links a user asks
// for: per address, so nobody gets a flood of them, and per account, so a
// signed-in user cannot make the site mail a new address each time.
func (s *Service) allowVerification(u store.User, addr string) error {
	if err := allow(s.verifyUser, strconv.FormatInt(u.ID, 10)); err != nil {
		return err
	}
	return allow(s.recoverEmail, strings.ToLower(addr))
}

// errNotVerifiable is returned when there is nothing to verify.
var errNotVerifiable = newError(http.StatusBadRequest, "no_email", "Add an email address first.", nil)

// errAlreadyVerified is returned when the address is verified and no
// change waits: the link was used, perhaps on another device, so the page
// that asked can send the user on to the course.
var errAlreadyVerified = newError(http.StatusConflict, "already_verified", "Your address is verified. Open your course.", nil)

// ResendVerification sends a new link for the pending or unverified
// address.
func (s *Service) ResendVerification(ctx context.Context, u store.User, c Client) (string, error) {
	addr, err := s.PendingEmail(ctx, u)
	if err != nil {
		return "", err
	}
	if addr == "" && !u.EmailVerified() {
		addr = u.Email
	}
	if addr == "" && u.EmailVerified() {
		return "", errAlreadyVerified
	}
	if addr == "" {
		return "", errNotVerifiable
	}
	if err := s.allowVerification(u, addr); err != nil {
		return "", err
	}
	if err := s.sendVerification(ctx, u, addr); err != nil {
		return "", err
	}
	s.audit(ctx, u.ID, "account.verification_resent", nil, c)
	return addr, nil
}
