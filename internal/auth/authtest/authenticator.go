// Package authtest is a software WebAuthn authenticator for tests: it
// answers the options of a begin endpoint with the JSON a browser would
// send to the finish endpoint. It makes ES256 discoverable credentials with
// "none" attestation, like a platform authenticator, and lets a test
// control the signature counter.
//
// It is for tests only. Its keys live in memory and it checks nothing a
// real authenticator would ask the user about.
package authtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/fxamacker/cbor/v2"
)

// Authenticator is a software authenticator for 1 origin.
type Authenticator struct {
	Origin string // for example "https://go-learn.party"

	mu    sync.Mutex
	creds []*Credential
}

// Credential is a passkey the authenticator holds.
type Credential struct {
	ID         []byte
	RPID       string
	UserHandle []byte
	SignCount  uint32
	// CountStep is added to SignCount before each assertion. 0 makes the
	// counter stay at 0, as synced passkeys do.
	CountStep uint32
	key       *ecdsa.PrivateKey
}

// New returns an authenticator for origin.
func New(origin string) *Authenticator { return &Authenticator{Origin: origin} }

var b64 = base64.RawURLEncoding

// creationOptions is the part of PublicKeyCredentialCreationOptions the
// authenticator needs. Begin endpoints wrap it as {"publicKey": ...}.
type creationOptions struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		ExcludeCredentials []struct {
			ID string `json:"id"`
		} `json:"excludeCredentials"`
		Extensions map[string]any `json:"extensions"`
	} `json:"publicKey"`
}

type requestOptions struct {
	PublicKey struct {
		Challenge        string `json:"challenge"`
		RPID             string `json:"rpId"`
		AllowCredentials []struct {
			ID string `json:"id"`
		} `json:"allowCredentials"`
	} `json:"publicKey"`
}

// ErrExcluded is returned when the authenticator holds an excluded
// credential, as a browser's InvalidStateError.
var ErrExcluded = errors.New("authtest: the authenticator holds an excluded credential")

// Flags of the authenticator data.
const (
	flagUP = 0x01
	flagUV = 0x04
	flagAT = 0x40
)

// Create answers creation options (the JSON of a begin endpoint's
// "options") with a new credential as the browser's PublicKeyCredential
// JSON.
func (a *Authenticator) Create(options []byte) (json.RawMessage, *Credential, error) {
	var o creationOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, nil, fmt.Errorf("authtest: creation options: %w", err)
	}
	pk := o.PublicKey
	handle, err := b64.DecodeString(pk.User.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("authtest: user.id: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ex := range pk.ExcludeCredentials {
		for _, c := range a.creds {
			if b64.EncodeToString(c.ID) == ex.ID {
				return nil, nil, ErrExcluded
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	cred := &Credential{ID: make([]byte, 16), RPID: pk.RP.ID, UserHandle: handle, key: key}
	_, _ = rand.Read(cred.ID)

	clientData, err := json.Marshal(map[string]any{
		"type": "webauthn.create", "challenge": pk.Challenge, "origin": a.Origin, "crossOrigin": false,
	})
	if err != nil {
		return nil, nil, err
	}
	point, err := key.PublicKey.Bytes() // 0x04 || X || Y, each 32 bytes
	if err != nil {
		return nil, nil, err
	}
	coseKey, err := cbor.Marshal(map[int]any{
		1:  2,  // kty: EC2
		3:  -7, // alg: ES256
		-1: 1,  // crv: P-256
		-2: point[1:33],
		-3: point[33:65],
	})
	if err != nil {
		return nil, nil, err
	}
	authData := authenticatorData(cred.RPID, flagUP|flagUV|flagAT, 0)
	authData = append(authData, make([]byte, 16)...) // AAGUID: zero, as with "none" attestation
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(cred.ID)))
	authData = append(authData, cred.ID...)
	authData = append(authData, coseKey...)
	attObj, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		return nil, nil, err
	}

	ext := map[string]any{}
	if _, ok := pk.Extensions["credProps"]; ok {
		ext["credProps"] = map[string]bool{"rk": true}
	}
	out, err := json.Marshal(map[string]any{
		"id":                      b64.EncodeToString(cred.ID),
		"rawId":                   b64.EncodeToString(cred.ID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientData),
			"attestationObject": b64.EncodeToString(attObj),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": ext,
	})
	if err != nil {
		return nil, nil, err
	}
	a.creds = append(a.creds, cred)
	return out, cred, nil
}

// Get answers request options with an assertion from the newest matching
// credential: one in allowCredentials, or for a discoverable login any
// credential of the relying party.
func (a *Authenticator) Get(options []byte) (json.RawMessage, error) {
	return a.GetWith(options, nil)
}

// GetWith answers with the credential that has the given ID; nil picks
// like Get.
func (a *Authenticator) GetWith(options []byte, id []byte) (json.RawMessage, error) {
	var o requestOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, fmt.Errorf("authtest: request options: %w", err)
	}
	pk := o.PublicKey
	a.mu.Lock()
	defer a.mu.Unlock()
	var cred *Credential
	for i := len(a.creds) - 1; i >= 0; i-- {
		c := a.creds[i]
		if c.RPID != pk.RPID || id != nil && string(c.ID) != string(id) {
			continue
		}
		if len(pk.AllowCredentials) > 0 && !allowed(c, pk.AllowCredentials) {
			continue
		}
		cred = c
		break
	}
	if cred == nil {
		return nil, errors.New("authtest: no matching credential")
	}
	cred.SignCount += cred.CountStep
	return a.assert(cred, pk.Challenge)
}

func allowed(c *Credential, list []struct {
	ID string `json:"id"`
}) bool {
	for _, a := range list {
		if a.ID == b64.EncodeToString(c.ID) {
			return true
		}
	}
	return false
}

// Replay signs an assertion with the counter set to count, as a cloned
// authenticator that lags behind would.
func (a *Authenticator) Replay(options []byte, c *Credential, count uint32) (json.RawMessage, error) {
	var o requestOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, fmt.Errorf("authtest: request options: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	saved := c.SignCount
	c.SignCount = count
	defer func() { c.SignCount = saved }()
	return a.assert(c, o.PublicKey.Challenge)
}

func (a *Authenticator) assert(cred *Credential, challenge string) (json.RawMessage, error) {
	clientData, err := json.Marshal(map[string]any{
		"type": "webauthn.get", "challenge": challenge, "origin": a.Origin, "crossOrigin": false,
	})
	if err != nil {
		return nil, err
	}
	authData := authenticatorData(cred.RPID, flagUP|flagUV, cred.SignCount)
	cdHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, cred.key, digest[:])
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id":                      b64.EncodeToString(cred.ID),
		"rawId":                   b64.EncodeToString(cred.ID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientData),
			"authenticatorData": b64.EncodeToString(authData),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(cred.UserHandle),
		},
		"clientExtensionResults": map[string]any{},
	})
}

// Credentials returns the credentials the authenticator holds.
func (a *Authenticator) Credentials() []*Credential {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]*Credential(nil), a.creds...)
}

// Forget drops every credential, as a lost device.
func (a *Authenticator) Forget() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creds = nil
}

func authenticatorData(rpID string, flags byte, count uint32) []byte {
	h := sha256.Sum256([]byte(rpID))
	b := append([]byte{}, h[:]...)
	b = append(b, flags)
	return binary.BigEndian.AppendUint32(b, count)
}
