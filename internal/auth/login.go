package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/yornik/go-practice-web/internal/store"
)

// BeginLogin starts a discoverable login: the browser offers the passkeys
// it has for the site, so the user types no username. conditional asks
// for the autofill flavour (mediation "conditional"), which a page starts
// on load and which waits until the user picks a passkey.
func (s *Service) BeginLogin(ctx context.Context, conditional bool, c Client) (Begin, error) {
	if err := allow(s.beginLimit, c.RateKey()); err != nil {
		return Begin{}, err
	}
	mediation := protocol.MediationDefault
	if conditional {
		mediation = protocol.MediationConditional
	}
	assertion, session, err := s.wa.BeginDiscoverableMediatedLogin(mediation)
	if err != nil {
		return Begin{}, fmt.Errorf("auth: begin login: %w", err)
	}
	id, err := s.saveCeremony(ctx, store.CeremonyLogin, 0, ceremonyData{Session: *session})
	if err != nil {
		return Begin{}, err
	}
	return Begin{Ceremony: id, Options: assertion}, nil
}

var (
	errUnknownPasskey = newError(http.StatusBadRequest, "unknown_passkey",
		"This passkey is not registered here. It may have been removed. Use another passkey or recover your account.", nil)
	errClonedPasskey = newError(http.StatusForbidden, "passkey_cloned",
		"This passkey reported a lower use counter than before, which can mean it was copied. "+
			"Sign in with another passkey or recover your account, then remove this one.", nil)
)

// FinishLogin verifies the assertion and returns the user. The caller
// starts the session.
func (s *Service) FinishLogin(ctx context.Context, in Finish, c Client) (store.User, error) {
	_, d, err := s.takeCeremony(ctx, in.Ceremony, store.CeremonyLogin)
	if err != nil {
		return store.User{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
	if err != nil {
		return store.User{}, newError(http.StatusBadRequest, "bad_credential", "The browser sent a passkey this site cannot read.", err)
	}

	var user store.User
	lookup := func(_, handle []byte) (webauthn.User, error) {
		u, err := s.store.UserByHandle(ctx, handle)
		if err != nil {
			return nil, err
		}
		user = u
		return s.loadWAUser(ctx, u)
	}
	_, cred, err := s.wa.ValidatePasskeyLogin(lookup, d.Session, parsed)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || isUnknownCredential(err) {
			s.audit(ctx, 0, "login.unknown_passkey", nil, c)
			return store.User{}, errUnknownPasskey
		}
		if user.ID != 0 {
			s.audit(ctx, user.ID, "login.failed", map[string]string{"reason": err.Error()}, c)
		}
		return store.User{}, newError(http.StatusBadRequest, "passkey_rejected", "The passkey could not be verified. Try again.", err)
	}
	if user.Disabled() {
		s.audit(ctx, user.ID, "login.disabled", nil, c)
		return store.User{}, errDisabled
	}

	use := store.CredentialUse{
		SignCount:    cred.Authenticator.SignCount,
		CloneWarning: cred.Authenticator.CloneWarning,
		BackupState:  cred.Flags.BackupState,
		UserVerified: cred.Flags.UserVerified,
	}
	if err := s.store.CredentialUsed(ctx, cred.ID, use); err != nil {
		return store.User{}, fmt.Errorf("auth: record login: %w", err)
	}
	if cred.Authenticator.CloneWarning {
		// A counter that does not grow means 2 copies of the key may be in
		// use. Synced passkeys always report 0, which never trips this.
		s.audit(ctx, user.ID, "login.clone_warning", map[string]any{"sign_count": parsed.Response.AuthenticatorData.Counter}, c)
		return store.User{}, errClonedPasskey
	}
	s.audit(ctx, user.ID, "login", nil, c)
	return user, nil
}

// isUnknownCredential reports whether go-webauthn rejected the assertion
// because the credential ID is not one of the user's: the passkey was
// removed here but is still on the device.
func isUnknownCredential(err error) bool {
	var unknown *protocol.ErrorUnknownCredential
	if errors.As(err, &unknown) {
		return true
	}
	var perr *protocol.Error
	return errors.As(err, &perr) && strings.Contains(perr.Details, "Unable to find the credential")
}
