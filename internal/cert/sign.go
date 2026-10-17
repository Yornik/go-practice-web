package cert

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Stats is the optional activity block of a certificate. The learner
// chooses whether it is shown; it is part of the signed payload when it is.
type Stats struct {
	ActiveSeconds int64           `json:"activeSeconds"`
	Runs          int             `json:"runs"`
	Hints         int             `json:"hints"` // hint levels 1 to 3 revealed, over all exercises
	Explanations  int             `json:"explanations"`
	Walkthroughs  int             `json:"walkthroughs"`
	Solutions     int             `json:"solutions"`
	Exercises     []ExerciseStats `json:"exercises"`
}

// ExerciseStats is the activity on 1 exercise.
type ExerciseStats struct {
	Slug          string `json:"slug"`
	ActiveSeconds int64  `json:"activeSeconds"`
	Runs          int    `json:"runs"`
	Hints         int    `json:"hints"`
	Explanation   bool   `json:"explanation"`
	Walkthrough   bool   `json:"walkthrough"`
	Solution      bool   `json:"solution"`
}

// Activity is what the store knows about 1 exercise of a user.
type Activity struct {
	ActiveSeconds int64
	Runs          int
	Levels        []int // the revealed hint ladder levels, 1 to 6
}

// Hint ladder levels, as course.Level numbers them.
const (
	levelExplanation = 4
	levelWalkthrough = 5
	levelSolution    = 6
)

// BuildStats sums the activity on the given exercises, in their order.
// Exercises without activity count as zero.
func BuildStats(slugs []string, activity map[string]Activity) *Stats {
	s := &Stats{Exercises: make([]ExerciseStats, 0, len(slugs))}
	for _, slug := range slugs {
		a := activity[slug]
		e := ExerciseStats{Slug: slug, ActiveSeconds: a.ActiveSeconds, Runs: a.Runs}
		for _, l := range a.Levels {
			switch {
			case l >= 1 && l <= 3:
				e.Hints++
			case l == levelExplanation:
				e.Explanation = true
			case l == levelWalkthrough:
				e.Walkthrough = true
			case l == levelSolution:
				e.Solution = true
			}
		}
		s.ActiveSeconds += e.ActiveSeconds
		s.Runs += e.Runs
		s.Hints += e.Hints
		s.Explanations += btoi(e.Explanation)
		s.Walkthroughs += btoi(e.Walkthrough)
		s.Solutions += btoi(e.Solution)
		s.Exercises = append(s.Exercises, e)
	}
	return s
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Payload is the signed content of a certificate.
type Payload struct {
	ID        string   `json:"id"`
	Kind      Kind     `json:"kind"`
	Name      string   `json:"name"`
	IssuedAt  string   `json:"issuedAt"` // RFC 3339, UTC, whole seconds
	Exercises []string `json:"exercises"`
	Stats     *Stats   `json:"stats,omitempty"`
}

// Issued parses IssuedAt.
func (p Payload) Issued() (time.Time, error) {
	t, err := time.Parse(time.RFC3339, p.IssuedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("cert: issuedAt: %w", err)
	}
	return t, nil
}

// Canonical returns the bytes that are signed: compact JSON with the
// fields in the declared order and no HTML escaping. The same payload
// always gives the same bytes.
func (p Payload) Canonical() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		return nil, fmt.Errorf("cert: encode payload: %w", err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// ParsePayload decodes canonical payload bytes. It rejects unknown fields
// and bytes that are not the canonical form, so that what the verify page
// shows is exactly what was signed.
func ParsePayload(b []byte) (Payload, error) {
	var p Payload
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Payload{}, fmt.Errorf("cert: decode payload: %w", err)
	}
	again, err := p.Canonical()
	if err != nil {
		return Payload{}, err
	}
	if !bytes.Equal(again, b) {
		return Payload{}, errors.New("cert: payload is not canonical")
	}
	return p, nil
}

// KeyStore keeps the signing key. *store.Store implements it.
type KeyStore interface {
	// SetSettingIfAbsent stores value under key unless key is set, and
	// returns the stored value.
	SetSettingIfAbsent(ctx context.Context, key, value string) (string, error)
}

// keySetting is the settings key of the private key seed (base64).
const keySetting = "cert.ed25519.seed"

// Signer signs and verifies certificates with an Ed25519 key.
type Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	now  func() time.Time
}

// LoadSigner returns the signer with the key from ks, generating and
// storing a key the first time. 2 servers that start at once end up with
// the same key, because the store keeps the first value written.
func LoadSigner(ctx context.Context, ks KeyStore) (*Signer, error) {
	seed := make([]byte, ed25519.SeedSize)
	// crypto/rand.Read never fails on supported platforms.
	_, _ = rand.Read(seed)
	stored, err := ks.SetSettingIfAbsent(ctx, keySetting, base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		return nil, fmt.Errorf("cert: load signing key: %w", err)
	}
	b, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return nil, fmt.Errorf("cert: stored signing key: %w", err)
	}
	return NewSigner(b)
}

// NewSigner returns a signer for a 32-byte Ed25519 seed.
func NewSigner(seed []byte) (*Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("cert: signing key seed has %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return &Signer{priv: priv, pub: priv.Public().(ed25519.PublicKey), now: time.Now}, nil
}

// PublicKey returns the verification key.
func (s *Signer) PublicKey() ed25519.PublicKey { return slices.Clone(s.pub) }

// PublicKeyBase64 returns the public key as the site serves it at
// /.well-known/go-practice-certificates.pub: the raw 32 bytes, base64.
func (s *Signer) PublicKeyBase64() string { return base64.StdEncoding.EncodeToString(s.pub) }

// Fingerprint returns the short fingerprint of the signing key.
func (s *Signer) Fingerprint() string { return Fingerprint(s.pub) }

// Fingerprint is the first 8 bytes of the SHA-256 of the raw public key,
// as 4 groups of 4 hex digits: "3F2A 91C4 0B7E D215". It is short enough to
// compare by eye between the PDF, the verify page and the published key.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	h := strings.ToUpper(hex.EncodeToString(sum[:8]))
	return h[0:4] + " " + h[4:8] + " " + h[8:12] + " " + h[12:16]
}

// Request is what a learner claims.
type Request struct {
	Kind      Kind
	Name      string    // cleaned with CleanName
	Exercises []string  // the slugs the kind covers, from Exercises
	Stats     *Stats    // nil hides the activity
	IssuedAt  time.Time // zero means now
}

// Certificate is a signed certificate, ready to store.
type Certificate struct {
	Payload   Payload
	Canonical []byte // the signed bytes
	Signature []byte
}

// Issue draws a new ID, builds the payload and signs it. If the store
// reports an ID collision, call Issue again.
func (s *Signer) Issue(r Request) (Certificate, error) {
	name, err := CleanName(r.Name)
	if err != nil {
		return Certificate{}, err
	}
	if len(r.Exercises) == 0 {
		return Certificate{}, errors.New("cert: a certificate needs exercises")
	}
	at := r.IssuedAt
	if at.IsZero() {
		at = s.now()
	}
	p := Payload{
		ID:        NewID(),
		Kind:      r.Kind,
		Name:      name,
		IssuedAt:  at.UTC().Truncate(time.Second).Format(time.RFC3339),
		Exercises: slices.Clone(r.Exercises),
		Stats:     r.Stats,
	}
	b, err := p.Canonical()
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{Payload: p, Canonical: b, Signature: ed25519.Sign(s.priv, b)}, nil
}

// ErrSignature is returned when a signature does not match the payload.
var ErrSignature = errors.New("cert: signature does not match")

// Verify checks a stored certificate and returns its payload.
func (s *Signer) Verify(canonical, sig []byte) (Payload, error) {
	return Verify(s.pub, canonical, sig)
}

// Verify checks canonical against sig with pub and returns the payload.
func Verify(pub ed25519.PublicKey, canonical, sig []byte) (Payload, error) {
	if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, canonical, sig) {
		return Payload{}, ErrSignature
	}
	return ParsePayload(canonical)
}
