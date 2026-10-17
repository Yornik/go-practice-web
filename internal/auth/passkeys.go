package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

// BeginAddPasskey starts the ceremony that adds a passkey to a signed-in
// user. The user's passkeys are excluded, so an authenticator that holds
// one already says so instead of making a duplicate.
func (s *Service) BeginAddPasskey(ctx context.Context, u store.User, c Client) (Begin, error) {
	if err := allow(s.beginLimit, c.RateKey()); err != nil {
		return Begin{}, err
	}
	wu, err := s.loadWAUser(ctx, u)
	if err != nil {
		return Begin{}, fmt.Errorf("auth: begin add passkey: %w", err)
	}
	creation, session, err := s.wa.BeginRegistration(wu, registrationOptions(wu.creds)...)
	if err != nil {
		return Begin{}, fmt.Errorf("auth: begin add passkey: %w", err)
	}
	id, err := s.saveCeremony(ctx, store.CeremonyAdd, u.ID, ceremonyData{Session: *session})
	if err != nil {
		return Begin{}, err
	}
	return Begin{Ceremony: id, Options: creation}, nil
}

// AddPasskey is the finish request of BeginAddPasskey: the credential and
// an optional label.
type AddPasskey struct {
	Finish
	Name string `json:"name"`
}

// FinishAddPasskey verifies and stores the new passkey and tells the user
// by email.
func (s *Service) FinishAddPasskey(ctx context.Context, u store.User, in AddPasskey, c Client) (store.Credential, error) {
	name, err := passkeyName(in.Name, c.UserAgent)
	if err != nil {
		return store.Credential{}, err
	}
	cer, d, err := s.takeCeremony(ctx, in.Ceremony, store.CeremonyAdd)
	if err != nil {
		return store.Credential{}, err
	}
	// A ceremony belongs to the session that started it; another user's
	// ID would add a passkey to the wrong account.
	if cer.UserID != u.ID {
		return store.Credential{}, errCeremony
	}
	wu, err := s.loadWAUser(ctx, u)
	if err != nil {
		return store.Credential{}, fmt.Errorf("auth: finish add passkey: %w", err)
	}
	cred, err := s.createCredential(wu, d.Session, in.Credential)
	if err != nil {
		return store.Credential{}, err
	}
	sc := toStore(cred, u.ID, name)
	if err := s.store.AddCredential(ctx, sc); err != nil {
		return store.Credential{}, storeError(err, "add passkey")
	}
	s.audit(ctx, u.ID, "passkey.added", map[string]string{"name": name}, c)
	if u.EmailVerified() {
		s.notify(u.Email, mail.PasskeyAdded{Name: u.DisplayName, Passkey: name, When: s.now()}, "passkey-added")
	}
	return sc, nil
}

// Passkeys returns the user's passkeys for the account page.
func (s *Service) Passkeys(ctx context.Context, u store.User) ([]store.Credential, error) {
	creds, err := s.store.Credentials(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: passkeys: %w", err)
	}
	return creds, nil
}

// RenamePasskey changes the label of 1 of the user's passkeys.
func (s *Service) RenamePasskey(ctx context.Context, u store.User, id []byte, name string, c Client) error {
	clean, err := cleanLabel(name, maxPasskeyRunes, "bad_passkey_name", "Use 1 to 64 printable characters for the passkey name.")
	if err != nil {
		return err
	}
	if err := s.store.RenameCredential(ctx, u.ID, id, clean); err != nil {
		return storeError(err, "rename passkey")
	}
	s.audit(ctx, u.ID, "passkey.renamed", map[string]string{"name": clean}, c)
	return nil
}

// DeletePasskey removes 1 of the user's passkeys, but never the last one.
func (s *Service) DeletePasskey(ctx context.Context, u store.User, id []byte, c Client) error {
	cred, err := s.store.Credential(ctx, id)
	if err != nil || cred.UserID != u.ID {
		return newError(http.StatusNotFound, "not_found", "This passkey does not exist.", err)
	}
	err = s.store.DeleteCredential(ctx, u.ID, id)
	if errors.Is(err, store.ErrLastCredential) {
		return newError(http.StatusConflict, "last_passkey",
			"This is your only passkey. Add another one before you remove it.", err)
	}
	if err != nil {
		return storeError(err, "delete passkey")
	}
	s.audit(ctx, u.ID, "passkey.removed", map[string]string{"name": cred.Name}, c)
	if u.EmailVerified() {
		s.notify(u.Email, mail.PasskeyRemoved{Name: u.DisplayName, Passkey: cred.Name, When: s.now()}, "passkey-removed")
	}
	return nil
}
