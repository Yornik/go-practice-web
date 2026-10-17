package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Session is a signed-in browser. Only the SHA-256 of the cookie token is
// stored, so a copy of the database does not sign anyone in.
type Session struct {
	TokenHash  []byte
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
}

// maxUserAgent caps the stored User-Agent header, which the client controls.
const maxUserAgent = 256

const sessionCols = `token_hash, user_id, created_at, expires_at, last_seen_at, user_agent`

func scanSession(r scanner) (Session, error) {
	var se Session
	err := r.Scan(&se.TokenHash, &se.UserID, tcol(&se.CreatedAt), tcol(&se.ExpiresAt), tcol(&se.LastSeenAt), &se.UserAgent)
	return se, mapErr(err)
}

// CreateSession stores a session that expires after ttl.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID int64, userAgent string, ttl time.Duration) (Session, error) {
	now := s.now().UTC().Truncate(time.Millisecond)
	if len(userAgent) > maxUserAgent {
		userAgent = userAgent[:maxUserAgent]
	}
	se := Session{TokenHash: tokenHash, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl), LastSeenAt: now, UserAgent: userAgent}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (`+sessionCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
		se.TokenHash, se.UserID, fmtTime(se.CreatedAt), fmtTime(se.ExpiresAt), fmtTime(se.LastSeenAt), se.UserAgent)
	if err != nil {
		return Session{}, fmt.Errorf("store: create session: %w", mapErr(err))
	}
	return se, nil
}

// SessionUser returns an unexpired session and its user in 1 query, for
// the request middleware. An expired or unknown token gives ErrNotFound.
// The caller still checks User.Disabled.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (Session, User, error) {
	var se Session
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT s.token_hash, s.user_id, s.created_at, s.expires_at, s.last_seen_at,
		s.user_agent, u.id, u.username, u.display_name, u.email, u.email_verified_at, u.is_admin, u.disabled_at,
		u.webauthn_handle, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, s.stamp()).Scan(
		&se.TokenHash, &se.UserID, tcol(&se.CreatedAt), tcol(&se.ExpiresAt), tcol(&se.LastSeenAt), &se.UserAgent,
		&u.ID, &u.Username, &u.DisplayName, scol(&u.Email), tcol(&u.EmailVerifiedAt), &u.IsAdmin, tcol(&u.DisabledAt),
		&u.WebAuthnHandle, tcol(&u.CreatedAt))
	if err != nil {
		return Session{}, User{}, fmt.Errorf("store: session: %w", mapErr(err))
	}
	return se, u, nil
}

// RefreshSession slides the expiry to now+ttl, but only when the session
// was last refreshed at least minInterval ago: a busy IDE makes many
// requests a minute, and each write is an fsync on NFS. It reports whether
// it wrote.
func (s *Store) RefreshSession(ctx context.Context, tokenHash []byte, ttl, minInterval time.Duration) (bool, error) {
	now := s.now()
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ?
		WHERE token_hash = ? AND expires_at > ? AND last_seen_at <= ?`,
		fmtTime(now), fmtTime(now.Add(ttl)), tokenHash, fmtTime(now), fmtTime(now.Add(-minInterval)))
	if err != nil {
		return false, fmt.Errorf("store: refresh session: %w", err)
	}
	n, err := rowsAffected(res)
	if err != nil {
		return false, fmt.Errorf("store: refresh session: %w", err)
	}
	return n == 1, nil
}

// DeleteSession signs a session out. An unknown token is not an error:
// logging out twice is fine.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteUserSessions signs a user out everywhere except the session with
// the hash keep (nil keeps none). It returns the number of sessions ended.
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keep []byte) (int64, error) {
	var res sql.Result
	var err error
	if keep == nil {
		res, err = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	} else {
		res, err = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keep)
	}
	if err != nil {
		return 0, fmt.Errorf("store: delete sessions of user %d: %w", userID, err)
	}
	return rowsAffected(res)
}

// Sessions returns a user's unexpired sessions, most recently seen first.
func (s *Store) Sessions(ctx context.Context, userID int64) ([]Session, error) {
	out, err := queryAll(ctx, s.db, scanSession, `SELECT `+sessionCols+` FROM sessions WHERE user_id = ? AND expires_at > ?
		ORDER BY last_seen_at DESC`, userID, s.stamp())
	if err != nil {
		return nil, fmt.Errorf("store: sessions: %w", err)
	}
	return out, nil
}

// Ceremony kinds: the WebAuthn flows of docs/DESIGN.md.
const (
	CeremonyRegister = "register"
	CeremonyLogin    = "login"
	CeremonyAdd      = "add"
	CeremonyRecover  = "recover"
)

// Ceremony is the server half of a WebAuthn begin/finish pair: the
// go-webauthn SessionData as JSON, kept between the 2 requests.
type Ceremony struct {
	ID        string // random, sent to the browser in a cookie or the response
	Kind      string // a Ceremony* constant
	UserID    int64  // 0 when no user is known yet (sign-up, login)
	Data      []byte // JSON
	ExpiresAt time.Time
}

// SaveCeremony stores a ceremony.
func (s *Store) SaveCeremony(ctx context.Context, c Ceremony) error {
	var user any
	if c.UserID != 0 {
		user = c.UserID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webauthn_ceremonies (id, kind, user_id, data, expires_at)
		VALUES (?, ?, ?, ?, ?)`, c.ID, c.Kind, user, string(c.Data), fmtTime(c.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: save ceremony: %w", mapErr(err))
	}
	return nil
}

// TakeCeremony removes and returns a ceremony of the given kind, so that a
// finish request can use it once. An expired, unknown or other-kind
// ceremony gives ErrNotFound.
func (s *Store) TakeCeremony(ctx context.Context, id, kind string) (Ceremony, error) {
	var c Ceremony
	var user sql.NullInt64
	var data string
	err := s.db.QueryRowContext(ctx, `DELETE FROM webauthn_ceremonies WHERE id = ? AND kind = ?
		RETURNING id, kind, user_id, data, expires_at`, id, kind).Scan(&c.ID, &c.Kind, &user, &data, tcol(&c.ExpiresAt))
	if err != nil {
		return Ceremony{}, fmt.Errorf("store: take ceremony: %w", mapErr(err))
	}
	if !c.ExpiresAt.After(s.now()) {
		return Ceremony{}, fmt.Errorf("store: take ceremony: expired: %w", ErrNotFound)
	}
	c.UserID = user.Int64
	c.Data = []byte(data)
	return c, nil
}

// CleanupResult counts what Cleanup deleted.
type CleanupResult struct {
	Sessions    int64
	Ceremonies  int64
	EmailTokens int64
}

// emailTokenRetention keeps used and expired email tokens for the audit
// trail of a recent recovery, then drops them.
const emailTokenRetention = 30 * 24 * time.Hour

// Cleanup deletes expired sessions and ceremonies, and email tokens that
// expired more than 30 days ago. Run it periodically.
func (s *Store) Cleanup(ctx context.Context) (CleanupResult, error) {
	var r CleanupResult
	now := s.now()
	for _, q := range []struct {
		n    *int64
		sql  string
		args []any
	}{
		{&r.Sessions, `DELETE FROM sessions WHERE expires_at <= ?`, []any{fmtTime(now)}},
		{&r.Ceremonies, `DELETE FROM webauthn_ceremonies WHERE expires_at <= ?`, []any{fmtTime(now)}},
		{&r.EmailTokens, `DELETE FROM email_tokens WHERE expires_at <= ?`, []any{fmtTime(now.Add(-emailTokenRetention))}},
	} {
		res, err := s.db.ExecContext(ctx, q.sql, q.args...)
		if err != nil {
			return r, fmt.Errorf("store: cleanup: %w", err)
		}
		if *q.n, err = rowsAffected(res); err != nil {
			return r, fmt.Errorf("store: cleanup: %w", err)
		}
	}
	return r, nil
}
