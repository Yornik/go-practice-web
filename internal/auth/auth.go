// Package auth holds the accounts of the site: passkey sign-up, login and
// management with go-webauthn, cookie sessions, the setup link, invites,
// email verification, recovery, security notices and the rate limits in
// front of them. See "Authentication and accounts" in docs/DESIGN.md.
//
// The package is HTTP aware only where it must be: it owns the session
// cookie. Everything else takes plain values, so the handlers in
// internal/web stay thin and the flows are testable without a server.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/yornik/go-practice-web/internal/mail"
	"github.com/yornik/go-practice-web/internal/store"
)

// Lifetimes of the things this package hands out.
const (
	SessionTTL      = 30 * 24 * time.Hour
	SessionRefresh  = time.Hour // a session is extended at most this often
	ceremonyTTL     = 5 * time.Minute
	DefaultInvite   = 7 * 24 * time.Hour
	MaxInviteDays   = 30
	RecoveryTTL     = 15 * time.Minute
	VerifyTTL       = 48 * time.Hour
	mailTimeout     = 30 * time.Second
	maxNoteRunes    = 500
	maxPasskeyRunes = 64
)

// Config configures a Service.
type Config struct {
	// PublicURL is GPW_PUBLIC_URL, for example https://go-learn.party. It
	// is the WebAuthn origin, its host is the relying party ID, and every
	// link in an email starts with it.
	PublicURL string
	// Dev drops the __Host- cookie prefix and the Secure flag, so that
	// http://localhost works.
	Dev bool
	// Exercises is the number of exercises in the course, which the
	// invite mail names; 0 leaves the number out.
	Exercises int
	Logger    *slog.Logger     // nil discards
	Now       func() time.Time // nil is time.Now; tests move the clock
}

// Service runs the account flows. It is safe for concurrent use.
type Service struct {
	store     *store.Store
	mailer    *mail.Mailer
	wa        *webauthn.WebAuthn
	log       *slog.Logger
	now       func() time.Time
	publicURL string
	dev       bool
	exercises int

	beginLimit   *Limiter // WebAuthn begin endpoints, per IP
	recoverIP    *Limiter // recovery requests, per IP
	recoverEmail *Limiter // recovery requests and verification links, per address
	verifyUser   *Limiter // verification links a user asks for, per account

	// bg tracks the notices sent in the background, so that a shutdown can
	// wait for them.
	bg sync.WaitGroup
}

// New returns a Service. The mailer sends verification links and
// security notices.
func New(st *store.Store, mailer *mail.Mailer, cfg Config) (*Service, error) {
	u, err := url.Parse(cfg.PublicURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("auth: public URL %q is not an http(s) origin", cfg.PublicURL)
	}
	if u.Scheme == "http" && !cfg.Dev {
		return nil, fmt.Errorf("auth: public URL %q must use https outside dev mode", cfg.PublicURL)
	}
	origin := u.Scheme + "://" + u.Host
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "Go Practice",
		RPOrigins:     []string{origin},
		// Attestation proves the authenticator model; an invite-only
		// learning site has no policy that would use it, and asking for
		// it adds a privacy prompt in some browsers.
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn: %w", err)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store:        st,
		mailer:       mailer,
		wa:           wa,
		log:          log,
		now:          now,
		publicURL:    origin,
		exercises:    cfg.Exercises,
		dev:          cfg.Dev,
		beginLimit:   NewLimiter(20, time.Minute, now),
		recoverIP:    NewLimiter(5, time.Hour, now),
		recoverEmail: NewLimiter(3, time.Hour, now),
		verifyUser:   NewLimiter(5, time.Hour, now),
	}, nil
}

// PublicURL returns the site origin without a trailing slash.
func (s *Service) PublicURL() string { return s.publicURL }

// MailDelivers reports whether the site's mail reaches a mail server.
// When it does not, every link goes to the server log (mail.LogSender),
// and only the operator can pass it on.
func (s *Service) MailDelivers() bool { return s.mailer.Delivers() }

// Wait blocks until the notices sent in the background are out. Call it at
// shutdown, after the HTTP server stopped.
func (s *Service) Wait() { s.bg.Wait() }

// Client is who made a request, for rate limits, the audit log and the
// label of a new passkey.
type Client struct {
	IP        string
	UserAgent string
}

// RateKey is the key of the client's address in per-address rate limits;
// see the function RateKey.
func (c Client) RateKey() string { return RateKey(c.IP) }

// Error is a failure the user can act on. Status is the HTTP status the
// web layer answers with; Code is the stable machine code of the JSON
// error body; Message is shown to the user.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration // set for rate limits
	Err        error         // the cause, for logs; may be nil
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func newError(status int, code, msg string, cause error) *Error {
	return &Error{Status: status, Code: code, Message: msg, Err: cause}
}

// AsError returns err as an *Error when it is one.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

func badInput(code, msg string) *Error { return newError(http.StatusBadRequest, code, msg, nil) }

func rateLimited(retry time.Duration) *Error {
	return &Error{
		Status:     http.StatusTooManyRequests,
		Code:       "rate_limited",
		Message:    "Too many attempts. Wait " + WaitText(retry) + ", then try again.",
		RetryAfter: retry,
	}
}

// allow applies a limiter and returns the rate-limit error when the key is
// over it.
func allow(l *Limiter, key string) error {
	if ok, retry := l.Allow(key); !ok {
		return rateLimited(retry)
	}
	return nil
}

// audit records an event. A failure to write the audit log must not fail
// the action the user took, so it is logged and dropped.
func (s *Service) audit(ctx context.Context, userID int64, event string, detail any, c Client) {
	if err := s.store.Audit(ctx, userID, event, detail, c.IP); err != nil {
		s.log.Error("audit", "event", event, "user_id", userID, "err", err)
	}
}

// notify sends an email in the background. Notices must not slow down or
// fail the request that caused them, and a recovery request must take the
// same time whether or not the address exists.
func (s *Service) notify(to string, e mail.Email, what string) {
	if to == "" {
		return
	}
	s.bg.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
		defer cancel()
		if err := s.mailer.Send(ctx, to, e); err != nil {
			s.log.Error("send email", "email", what, "err", err)
			return
		}
		s.log.Info("email sent", "email", what)
	})
}

// link returns an absolute URL on the site.
func (s *Service) link(path string, query url.Values) string {
	u := s.publicURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// storeError maps the store's sentinel errors to user-facing errors and
// wraps anything else as an internal error.
func storeError(err error, what string) error {
	var conflict *store.ConflictError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &conflict):
		col := conflict.Column
		switch {
		case strings.HasSuffix(col, "username"):
			return newError(http.StatusConflict, "username_taken", "This username is taken. Choose another one.", err)
		case strings.HasSuffix(col, "email"):
			return newError(http.StatusConflict, "email_taken", "Another account uses this email address.", err)
		case strings.HasPrefix(col, "credentials."):
			return newError(http.StatusConflict, "passkey_exists", "This passkey is already registered.", err)
		}
		return newError(http.StatusConflict, "conflict", "This already exists.", err)
	case errors.Is(err, store.ErrTokenInvalid):
		return newError(http.StatusGone, "token_invalid", "This link was already used or has expired.", err)
	case errors.Is(err, store.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Not found.", err)
	}
	return fmt.Errorf("auth: %s: %w", what, err)
}
