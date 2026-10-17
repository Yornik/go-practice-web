package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Invite is a single-use sign-up link. Only the token's hash is stored.
type Invite struct {
	TokenHash []byte
	CreatedBy int64  // 0 when the creating admin was deleted
	Email     string // the address the admin sent it to; "" when copied by hand
	Note      string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedBy    int64     // 0 while unused
	UsedAt    time.Time // zero while unused
	// MailedAt is when an SMTP server accepted the invite mail; zero for
	// an invite without an address, a mail that failed or one that only
	// reached the log.
	MailedAt time.Time
}

// Used reports whether someone signed up with the invite.
func (i Invite) Used() bool { return !i.UsedAt.IsZero() }

// Mailed reports whether the invite went out by mail. Opening its link
// then proves control of Email.
func (i Invite) Mailed() bool { return i.Email != "" && !i.MailedAt.IsZero() }

// Expired reports whether the invite expired at now.
func (i Invite) Expired(now time.Time) bool { return !now.Before(i.ExpiresAt) }

const inviteCols = `token_hash, created_by, email, note, created_at, expires_at, used_by, used_at, mailed_at`

func scanInvite(r scanner) (Invite, error) {
	var i Invite
	var by, usedBy sql.NullInt64
	err := r.Scan(&i.TokenHash, &by, scol(&i.Email), &i.Note, tcol(&i.CreatedAt), tcol(&i.ExpiresAt), &usedBy, tcol(&i.UsedAt),
		tcol(&i.MailedAt))
	i.CreatedBy, i.UsedBy = by.Int64, usedBy.Int64
	return i, mapErr(err)
}

// CreateInvite stores an invite from TokenHash, CreatedBy, Email, Note and
// ExpiresAt; the store sets CreatedAt.
func (s *Store) CreateInvite(ctx context.Context, i Invite) (Invite, error) {
	i.CreatedAt = s.now().UTC().Truncate(time.Millisecond)
	var by any
	if i.CreatedBy != 0 {
		by = i.CreatedBy
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO invites (token_hash, created_by, email, note, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, i.TokenHash, by, nullString(strings.TrimSpace(i.Email)), i.Note,
		fmtTime(i.CreatedAt), fmtTime(i.ExpiresAt))
	if err != nil {
		return Invite{}, fmt.Errorf("store: create invite: %w", mapErr(err))
	}
	return i, nil
}

// MarkInviteMailed records that an SMTP server accepted the mail of an
// unused invite. Call it only after a real delivery: the mark makes a
// sign-up with the invite's address verified.
func (s *Store) MarkInviteMailed(ctx context.Context, hash []byte) error {
	res, err := s.db.ExecContext(ctx, `UPDATE invites SET mailed_at = ? WHERE token_hash = ? AND used_at IS NULL`, s.stamp(), hash)
	if err := found(res, err); err != nil {
		return fmt.Errorf("store: mark invite mailed: %w", err)
	}
	return nil
}

// Invite returns an invite in any state.
func (s *Store) Invite(ctx context.Context, hash []byte) (Invite, error) {
	i, err := scanInvite(s.db.QueryRowContext(ctx, `SELECT `+inviteCols+` FROM invites WHERE token_hash = ?`, hash))
	if err != nil {
		return Invite{}, fmt.Errorf("store: invite: %w", err)
	}
	return i, nil
}

// ValidInvite returns an invite that can still be used; otherwise
// ErrTokenInvalid. The sign-up page checks it before the passkey ceremony;
// RegisterWithInvite checks it again atomically.
func (s *Store) ValidInvite(ctx context.Context, hash []byte) (Invite, error) {
	i, err := s.Invite(ctx, hash)
	if errors.Is(err, ErrNotFound) || err == nil && (i.Used() || i.Expired(s.now())) {
		return Invite{}, fmt.Errorf("store: invite: %w", ErrTokenInvalid)
	}
	return i, err
}

// Invites returns every invite, newest first.
func (s *Store) Invites(ctx context.Context) ([]Invite, error) {
	out, err := queryAll(ctx, s.db, scanInvite, `SELECT `+inviteCols+` FROM invites ORDER BY created_at DESC, rowid DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: invites: %w", err)
	}
	return out, nil
}

// DeleteInvite revokes an unused invite. A used invite stays as the record
// of who invited whom: ErrNotFound.
func (s *Store) DeleteInvite(ctx context.Context, hash []byte) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE token_hash = ? AND used_at IS NULL`, hash)
	if err := found(res, err); err != nil {
		return fmt.Errorf("store: delete invite: %w", err)
	}
	return nil
}

// ReplaceSetupToken stores a new setup token and deletes unused ones, so
// that only the link logged last works. The web server calls it at start
// while no user exists: a restart before setup logs a fresh link.
func (s *Store) ReplaceSetupToken(ctx context.Context, hash []byte) error {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM setup_tokens WHERE used_at IS NULL`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO setup_tokens (token_hash, created_at) VALUES (?, ?)`, hash, s.stamp())
		return mapErr(err)
	})
	if err != nil {
		return fmt.Errorf("store: replace setup token: %w", err)
	}
	return nil
}

// HasUnusedSetupToken reports whether a setup link is waiting.
func (s *Store) HasUnusedSetupToken(ctx context.Context) (bool, error) {
	var ok bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM setup_tokens WHERE used_at IS NULL)`).Scan(&ok); err != nil {
		return false, fmt.Errorf("store: setup token: %w", err)
	}
	return ok, nil
}

// ValidSetupToken returns nil when hash is an unused setup token, else
// ErrTokenInvalid.
func (s *Store) ValidSetupToken(ctx context.Context, hash []byte) error {
	var ok bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM setup_tokens WHERE token_hash = ? AND used_at IS NULL)`,
		hash).Scan(&ok); err != nil {
		return fmt.Errorf("store: setup token: %w", err)
	}
	if !ok {
		return fmt.Errorf("store: setup token: %w", ErrTokenInvalid)
	}
	return nil
}

// Purposes of an EmailToken.
const (
	PurposeVerify  = "verify"
	PurposeRecover = "recover"
)

// EmailToken is a link sent by email: verify an address, or recover an
// account. Only the token's hash is stored.
type EmailToken struct {
	TokenHash []byte
	UserID    int64
	Purpose   string // PurposeVerify or PurposeRecover
	Email     string // the address the link went to; VerifyEmail sets it on the user
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time
}

const emailTokenCols = `token_hash, user_id, purpose, email, created_at, expires_at, used_at`

func scanEmailToken(r scanner) (EmailToken, error) {
	var t EmailToken
	err := r.Scan(&t.TokenHash, &t.UserID, &t.Purpose, &t.Email, tcol(&t.CreatedAt), tcol(&t.ExpiresAt), tcol(&t.UsedAt))
	return t, mapErr(err)
}

// CreateEmailToken stores a token from TokenHash, UserID, Purpose and Email
// that expires after ttl. It deletes the user's other unused tokens of the
// same purpose: only the newest link works.
func (s *Store) CreateEmailToken(ctx context.Context, t EmailToken, ttl time.Duration) (EmailToken, error) {
	now := s.now().UTC().Truncate(time.Millisecond)
	t.CreatedAt, t.ExpiresAt, t.UsedAt = now, now.Add(ttl), time.Time{}
	t.Email = strings.TrimSpace(t.Email)
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM email_tokens WHERE user_id = ? AND purpose = ? AND used_at IS NULL`,
			t.UserID, t.Purpose); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO email_tokens (token_hash, user_id, purpose, email, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)`, t.TokenHash, t.UserID, t.Purpose, t.Email, fmtTime(t.CreatedAt), fmtTime(t.ExpiresAt))
		return mapErr(err)
	})
	if err != nil {
		return EmailToken{}, fmt.Errorf("store: create email token: %w", err)
	}
	return t, nil
}

// ValidEmailToken returns an unused, unexpired token of the purpose, else
// ErrTokenInvalid. It does not use the token.
func (s *Store) ValidEmailToken(ctx context.Context, hash []byte, purpose string) (EmailToken, error) {
	t, err := scanEmailToken(s.db.QueryRowContext(ctx, `SELECT `+emailTokenCols+` FROM email_tokens
		WHERE token_hash = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?`, hash, purpose, s.stamp()))
	if errors.Is(err, ErrNotFound) {
		err = ErrTokenInvalid
	}
	if err != nil {
		return EmailToken{}, fmt.Errorf("store: email token: %w", err)
	}
	return t, nil
}

// useEmailToken marks a valid token used and returns it.
func (s *Store) useEmailToken(ctx context.Context, tx *sql.Tx, hash []byte, purpose string) (EmailToken, error) {
	now := s.stamp()
	t, err := scanEmailToken(tx.QueryRowContext(ctx, `UPDATE email_tokens SET used_at = ?
		WHERE token_hash = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?
		RETURNING `+emailTokenCols, now, hash, purpose, now))
	if errors.Is(err, ErrNotFound) {
		err = ErrTokenInvalid
	}
	return t, err
}

// UseEmailToken marks a valid token used and returns it; a second use
// gives ErrTokenInvalid.
func (s *Store) UseEmailToken(ctx context.Context, hash []byte, purpose string) (EmailToken, error) {
	var t EmailToken
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var err error
		t, err = s.useEmailToken(ctx, tx, hash, purpose)
		return err
	})
	if err != nil {
		return EmailToken{}, fmt.Errorf("store: use email token: %w", err)
	}
	return t, nil
}

// VerifyEmail uses a verify token and sets the token's address as the
// user's verified email. If another account verified that address in the
// meantime, it gives a *ConflictError and the token stays unused.
func (s *Store) VerifyEmail(ctx context.Context, hash []byte) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		t, err := s.useEmailToken(ctx, tx, hash, PurposeVerify)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET email = ?, email_verified_at = ? WHERE id = ?`,
			t.Email, s.stamp(), t.UserID); err != nil {
			return mapErr(err)
		}
		u, err = scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, t.UserID))
		return err
	})
	if err != nil {
		return User{}, fmt.Errorf("store: verify email: %w", err)
	}
	return u, nil
}

// PendingEmail returns the user's newest unused, unexpired verify token,
// so the account page can say "check your inbox at ...". ErrNotFound when
// there is none.
func (s *Store) PendingEmail(ctx context.Context, userID int64) (EmailToken, error) {
	t, err := scanEmailToken(s.db.QueryRowContext(ctx, `SELECT `+emailTokenCols+` FROM email_tokens
		WHERE user_id = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC LIMIT 1`, userID, PurposeVerify, s.stamp()))
	if err != nil {
		return EmailToken{}, fmt.Errorf("store: pending email: %w", err)
	}
	return t, nil
}

// RecoverAccount finishes a recovery: it uses the recover token, adds the
// new passkey and, if removeOthers, deletes the user's other passkeys and
// sessions, all or nothing. It returns the user and the number of passkeys
// removed.
func (s *Store) RecoverAccount(ctx context.Context, hash []byte, c Credential, removeOthers bool) (User, int64, error) {
	var u User
	var removed int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		t, err := s.useEmailToken(ctx, tx, hash, PurposeRecover)
		if err != nil {
			return err
		}
		c.UserID = t.UserID
		if err := insertCredential(ctx, tx, c, s.now()); err != nil {
			return err
		}
		if removeOthers {
			res, err := tx.ExecContext(ctx, `DELETE FROM credentials WHERE user_id = ? AND id != ?`, t.UserID, c.ID)
			if err != nil {
				return err
			}
			if removed, err = rowsAffected(res); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, t.UserID); err != nil {
				return err
			}
		}
		u, err = scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, t.UserID))
		return err
	})
	if err != nil {
		return User{}, 0, fmt.Errorf("store: recover account: %w", err)
	}
	return u, removed, nil
}
