package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/yornik/go-practice-web/internal/store"
)

// Begin is the first half of a WebAuthn ceremony: the options for
// navigator.credentials.create or .get, and the ID the browser sends back
// with the result.
type Begin struct {
	Ceremony string `json:"ceremony"`
	// Options is protocol.CredentialCreation or protocol.CredentialAssertion:
	// {"publicKey": {...}, "mediation": "..."}.
	Options any `json:"options"`
}

// Finish is the second half: the browser's PublicKeyCredential as JSON.
type Finish struct {
	Ceremony   string          `json:"ceremony"`
	Credential json.RawMessage `json:"credential"`
}

// waUser adapts a user to go-webauthn.
type waUser struct {
	handle      []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.handle }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadWAUser returns the user with their stored passkeys.
func (s *Service) loadWAUser(ctx context.Context, u store.User) (*waUser, error) {
	creds, err := s.store.Credentials(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	wu := &waUser{handle: u.WebAuthnHandle, name: u.Username, displayName: u.DisplayName}
	for _, c := range creds {
		wu.creds = append(wu.creds, fromStore(c))
	}
	return wu, nil
}

// fromStore rebuilds the go-webauthn credential record that a login is
// verified against.
func fromStore(c store.Credential) webauthn.Credential {
	var flags protocol.AuthenticatorFlags
	for _, f := range []struct {
		set  bool
		flag protocol.AuthenticatorFlags
	}{
		{c.UserPresent, protocol.FlagUserPresent},
		{c.UserVerified, protocol.FlagUserVerified},
		{c.BackupEligible, protocol.FlagBackupEligible},
		{c.BackupState, protocol.FlagBackupState},
	} {
		if f.set {
			flags |= f.flag
		}
	}
	transports := make([]protocol.AuthenticatorTransport, 0, len(c.Transports))
	for _, t := range c.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.ID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Flags:           webauthn.NewCredentialFlags(flags),
		Authenticator: webauthn.Authenticator{
			AAGUID:       c.AAGUID,
			SignCount:    c.SignCount,
			CloneWarning: c.CloneWarning,
			Attachment:   protocol.AuthenticatorAttachment(c.Attachment),
		},
	}
}

// toStore turns a verified new credential into the row the store keeps.
func toStore(c *webauthn.Credential, userID int64, name string) store.Credential {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}
	return store.Credential{
		ID:              c.ID,
		UserID:          userID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		AAGUID:          c.Authenticator.AAGUID,
		SignCount:       c.Authenticator.SignCount,
		CloneWarning:    c.Authenticator.CloneWarning,
		Transports:      transports,
		Attachment:      string(c.Authenticator.Attachment),
		UserPresent:     c.Flags.UserPresent,
		UserVerified:    c.Flags.UserVerified,
		BackupEligible:  c.Flags.BackupEligible,
		BackupState:     c.Flags.BackupState,
		Name:            name,
	}
}

// ceremonyData is what a ceremony keeps between begin and finish.
type ceremonyData struct {
	Session webauthn.SessionData `json:"session"`
	// Sign-up: the account to create and the token that allows it.
	User      *pendingUser `json:"user,omitempty"`
	TokenHash []byte       `json:"tokenHash,omitempty"` // invite, setup or recover token
	Setup     bool         `json:"setup,omitempty"`     // TokenHash is a setup token
}

type pendingUser struct {
	Handle      []byte `json:"handle"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// saveCeremony stores the server half of a ceremony and returns its ID.
func (s *Service) saveCeremony(ctx context.Context, kind string, userID int64, d ceremonyData) (string, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("auth: encode ceremony: %w", err)
	}
	id := rand.Text()
	err = s.store.SaveCeremony(ctx, store.Ceremony{
		ID: id, Kind: kind, UserID: userID, Data: data, ExpiresAt: s.now().Add(ceremonyTTL),
	})
	if err != nil {
		return "", fmt.Errorf("auth: save ceremony: %w", err)
	}
	return id, nil
}

var errCeremony = newError(http.StatusBadRequest, "ceremony_expired",
	"This passkey request expired or was already used. Start again.", nil)

// takeCeremony returns a ceremony once.
func (s *Service) takeCeremony(ctx context.Context, id, kind string) (store.Ceremony, ceremonyData, error) {
	if id == "" || len(id) > 64 {
		return store.Ceremony{}, ceremonyData{}, errCeremony
	}
	c, err := s.store.TakeCeremony(ctx, id, kind)
	if errors.Is(err, store.ErrNotFound) {
		return store.Ceremony{}, ceremonyData{}, errCeremony
	}
	if err != nil {
		return store.Ceremony{}, ceremonyData{}, fmt.Errorf("auth: take ceremony: %w", err)
	}
	var d ceremonyData
	if err := json.Unmarshal(c.Data, &d); err != nil {
		return store.Ceremony{}, ceremonyData{}, fmt.Errorf("auth: decode ceremony: %w", err)
	}
	return c, d, nil
}

// registrationOptions are the options of every create() call: a
// discoverable credential, so login needs no username, and the credProps
// extension, so the browser reports whether it made one.
func registrationOptions(exclude []webauthn.Credential) []webauthn.RegistrationOption {
	opts := []webauthn.RegistrationOption{
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()),
	}
	if len(exclude) > 0 {
		opts = append(opts, webauthn.WithExclusions(webauthn.Credentials(exclude).CredentialDescriptors()))
	}
	return opts
}

// createCredential verifies the browser's create() result.
func (s *Service) createCredential(user webauthn.User, session webauthn.SessionData, raw json.RawMessage) (*webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return nil, newError(http.StatusBadRequest, "bad_credential", "The browser sent a passkey this site cannot read.", err)
	}
	cred, err := s.wa.CreateCredential(user, session, parsed)
	if err != nil {
		return nil, newError(http.StatusBadRequest, "passkey_rejected", "The passkey could not be verified. Try again.", err)
	}
	return cred, nil
}

// passkeyName is the default label of a new passkey: the browser and the
// system it was made on, which is what a user recognises on the account
// page.
func passkeyName(name, userAgent string) (string, error) {
	if strings.TrimSpace(name) != "" {
		return cleanLabel(name, maxPasskeyRunes, "bad_passkey_name", "Use 1 to 64 printable characters for the passkey name.")
	}
	return deviceName(userAgent), nil
}

// deviceName guesses "Browser on System" from a User-Agent. The order of
// the checks matters: Edge and Chrome on Android also say "Safari", and
// iPhones also say "Mac OS X".
func deviceName(ua string) string {
	var system string
	switch {
	case strings.Contains(ua, "iPhone"):
		system = "iPhone"
	case strings.Contains(ua, "iPad"):
		system = "iPad"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		system = "macOS"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	var browser string
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case system != "":
		return system
	case browser != "":
		return browser
	}
	return "Passkey"
}
