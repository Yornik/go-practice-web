package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/yornik/go-practice-web/internal/store"
)

// ErrNoSession means the request carries no valid session: no cookie, an
// unknown or expired token, or a disabled account.
var ErrNoSession = errors.New("auth: no session")

// cookieName is the session cookie. The __Host- prefix makes the browser
// refuse the cookie unless it is Secure, has Path=/ and no Domain, so a
// sibling subdomain cannot set or shadow it. Browsers refuse that prefix
// over plain http, hence the dev name.
func (s *Service) cookieName() string {
	if s.dev {
		return "gpw_session"
	}
	return "__Host-gpw_session"
}

// Identity is the signed-in user of a request.
type Identity struct {
	User    store.User
	Session store.Session
}

// StartSession creates a session for the user and sets the cookie.
func (s *Service) StartSession(ctx context.Context, w http.ResponseWriter, userID int64, c Client) error {
	token, hash := newToken()
	if _, err := s.store.CreateSession(ctx, hash, userID, c.UserAgent, SessionTTL); err != nil {
		return fmt.Errorf("auth: start session: %w", err)
	}
	s.setCookie(w, token, int(SessionTTL.Seconds()))
	return nil
}

func (s *Service) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !s.dev,
		// Lax keeps the user signed in when they follow a link to the site
		// and still withholds the cookie from cross-site POSTs; the CSRF
		// header and Origin check are the first layer.
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) clearCookie(w http.ResponseWriter) { s.setCookie(w, "", -1) }

// Authenticate returns the identity of a request. It slides the session's
// expiry at most once per SessionRefresh and then renews the cookie, so
// an active user stays signed in. A disabled account has no session.
func (s *Service) Authenticate(ctx context.Context, w http.ResponseWriter, r *http.Request) (Identity, error) {
	ck, err := r.Cookie(s.cookieName())
	if err != nil {
		return Identity{}, ErrNoSession
	}
	hash, ok := hashToken(ck.Value)
	if !ok {
		s.clearCookie(w)
		return Identity{}, ErrNoSession
	}
	se, u, err := s.store.SessionUser(ctx, hash)
	if errors.Is(err, store.ErrNotFound) {
		s.clearCookie(w)
		return Identity{}, ErrNoSession
	}
	if err != nil {
		return Identity{}, fmt.Errorf("auth: session: %w", err)
	}
	if u.Disabled() {
		// SetDisabled deletes the sessions; this covers a race with it.
		_ = s.store.DeleteSession(ctx, hash)
		s.clearCookie(w)
		return Identity{}, ErrNoSession
	}
	refreshed, err := s.store.RefreshSession(ctx, hash, SessionTTL, SessionRefresh)
	if err != nil {
		// The session is valid for days; a failed refresh only shortens it.
		s.log.Warn("refresh session", "user_id", u.ID, "err", err)
	} else if refreshed {
		s.setCookie(w, ck.Value, int(SessionTTL.Seconds()))
	}
	return Identity{User: u, Session: se}, nil
}

// Logout ends the request's session, or every session of the user when
// everywhere is set, and clears the cookie.
func (s *Service) Logout(ctx context.Context, w http.ResponseWriter, id Identity, everywhere bool, c Client) error {
	s.clearCookie(w)
	if everywhere {
		n, err := s.store.DeleteUserSessions(ctx, id.User.ID, nil)
		if err != nil {
			return fmt.Errorf("auth: logout: %w", err)
		}
		s.audit(ctx, id.User.ID, "logout.everywhere", map[string]int64{"sessions": n}, c)
		return nil
	}
	if err := s.store.DeleteSession(ctx, id.Session.TokenHash); err != nil {
		return fmt.Errorf("auth: logout: %w", err)
	}
	s.audit(ctx, id.User.ID, "logout", nil, c)
	return nil
}
