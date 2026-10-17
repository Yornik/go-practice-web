package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// User is an account.
type User struct {
	ID              int64
	Username        string // 3 to 32 characters of [a-z0-9_-]; the schema enforces it
	DisplayName     string
	Email           string    // "" when the user has none
	EmailVerifiedAt time.Time // zero while Email is not verified
	IsAdmin         bool
	DisabledAt      time.Time // zero while the account is enabled
	WebAuthnHandle  []byte    // the WebAuthn user handle: random, never shown
	CreatedAt       time.Time
}

// Disabled reports whether an admin disabled the account.
func (u User) Disabled() bool { return !u.DisabledAt.IsZero() }

// EmailVerified reports whether Email is verified and so usable for
// recovery.
func (u User) EmailVerified() bool { return u.Email != "" && !u.EmailVerifiedAt.IsZero() }

// NewUser is the input for a registration.
type NewUser struct {
	Username       string
	DisplayName    string
	Email          string // stored unverified; "" for none
	WebAuthnHandle []byte
}

type scanner interface{ Scan(dest ...any) error }

const userCols = `id, username, display_name, email, email_verified_at, is_admin, disabled_at, webauthn_handle, created_at`

func scanUser(r scanner) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Username, &u.DisplayName, scol(&u.Email), tcol(&u.EmailVerifiedAt),
		&u.IsAdmin, tcol(&u.DisabledAt), &u.WebAuthnHandle, tcol(&u.CreatedAt))
	return u, mapErr(err)
}

func (s *Store) userWhere(ctx context.Context, what, where string, args ...any) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE `+where, args...))
	if err != nil {
		return User{}, fmt.Errorf("store: user by %s: %w", what, err)
	}
	return u, nil
}

// User returns the user with the given ID.
func (s *Store) User(ctx context.Context, id int64) (User, error) {
	return s.userWhere(ctx, "id", `id = ?`, id)
}

// UserByUsername returns the user with the given username.
func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	return s.userWhere(ctx, "username", `username = ?`, username)
}

// UserByHandle returns the user with the given WebAuthn user handle, which
// a discoverable login reports.
func (s *Store) UserByHandle(ctx context.Context, handle []byte) (User, error) {
	return s.userWhere(ctx, "handle", `webauthn_handle = ?`, handle)
}

// UserByVerifiedEmail returns the user whose verified email is email,
// ignoring case. Recovery uses it: an unverified address must not work.
func (s *Store) UserByVerifiedEmail(ctx context.Context, email string) (User, error) {
	return s.userWhere(ctx, "verified email", `email = ? AND email_verified_at IS NOT NULL`, strings.TrimSpace(email))
}

// Users returns every user, oldest first.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	out, err := queryAll(ctx, s.db, scanUser, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: users: %w", err)
	}
	return out, nil
}

// CountUsers returns the number of accounts. Setup runs while it is 0.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count users: %w", err)
	}
	return n, nil
}

func (s *Store) updateUser(ctx context.Context, what string, id int64, set string, args ...any) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET `+set+` WHERE id = ?`, append(args, id)...)
	if err != nil {
		return fmt.Errorf("store: set %s of user %d: %w", what, id, mapErr(err))
	}
	n, err := rowsAffected(res)
	if err != nil {
		return fmt.Errorf("store: set %s of user %d: %w", what, id, err)
	}
	if n == 0 {
		return fmt.Errorf("store: set %s of user %d: %w", what, id, ErrNotFound)
	}
	return nil
}

// SetDisplayName changes a user's display name.
func (s *Store) SetDisplayName(ctx context.Context, id int64, name string) error {
	return s.updateUser(ctx, "display name", id, `display_name = ?`, name)
}

// SetEmail sets an unverified email address ("" removes it). The usual way
// to change an address is VerifyEmail with a token for the new address,
// which keeps the old verified address usable until then.
func (s *Store) SetEmail(ctx context.Context, id int64, email string) error {
	return s.updateUser(ctx, "email", id, `email = ?, email_verified_at = NULL`, nullString(strings.TrimSpace(email)))
}

// SetDisabled disables or enables an account. Disabling also deletes its
// sessions, so the user is signed out at once.
func (s *Store) SetDisabled(ctx context.Context, id int64, disabled bool) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var res sql.Result
		var err error
		if disabled {
			res, err = tx.ExecContext(ctx, `UPDATE users SET disabled_at = COALESCE(disabled_at, ?) WHERE id = ?`, s.stamp(), id)
		} else {
			res, err = tx.ExecContext(ctx, `UPDATE users SET disabled_at = NULL WHERE id = ?`, id)
		}
		if err != nil {
			return fmt.Errorf("store: disable user %d: %w", id, err)
		}
		if n, err := rowsAffected(res); err != nil || n == 0 {
			return fmt.Errorf("store: disable user %d: %w", id, errors.Join(err, ErrNotFound))
		}
		if disabled {
			if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
				return fmt.Errorf("store: disable user %d: sessions: %w", id, err)
			}
		}
		return nil
	})
}

// SetAdmin grants or removes the admin flag.
func (s *Store) SetAdmin(ctx context.Context, id int64, admin bool) error {
	return s.updateUser(ctx, "admin", id, `is_admin = ?`, admin)
}

// Registration is a new account with its first passkey.
type Registration struct {
	User       NewUser
	Credential Credential // UserID is filled in
}

// RegisterWithInvite creates the account and its passkey and uses the
// invite, all or nothing. A used, expired or unknown invite gives
// ErrTokenInvalid; a taken username or email gives a *ConflictError.
func (s *Store) RegisterWithInvite(ctx context.Context, inviteHash []byte, r Registration) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		// The invite first, so that a used invite reports as such even when
		// the username is taken too.
		now := s.stamp()
		res, err := tx.ExecContext(ctx, `UPDATE invites SET used_at = ?
			WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`, now, inviteHash, now)
		if err := oneRow(res, err); err != nil {
			return err
		}
		if u, err = s.insertRegistration(ctx, tx, r, false); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE invites SET used_by = ? WHERE token_hash = ?`, u.ID, inviteHash)
		return err
	})
	if err != nil {
		return User{}, fmt.Errorf("store: register with invite: %w", err)
	}
	return u, nil
}

// oneRow turns the result of a conditional token update into
// ErrTokenInvalid when no row matched.
func oneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := rowsAffected(res)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrTokenInvalid
	}
	return nil
}

// RegisterWithSetupToken creates the first account as an admin and uses
// the setup token, all or nothing.
func (s *Store) RegisterWithSetupToken(ctx context.Context, setupHash []byte, r Registration) (User, error) {
	var u User
	err := s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE setup_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`,
			s.stamp(), setupHash)
		if err := oneRow(res, err); err != nil {
			return err
		}
		u, err = s.insertRegistration(ctx, tx, r, true)
		return err
	})
	if err != nil {
		return User{}, fmt.Errorf("store: register with setup token: %w", err)
	}
	return u, nil
}

func (s *Store) insertRegistration(ctx context.Context, tx *sql.Tx, r Registration, admin bool) (User, error) {
	now := s.now()
	u := User{
		Username:       r.User.Username,
		DisplayName:    r.User.DisplayName,
		Email:          strings.TrimSpace(r.User.Email),
		IsAdmin:        admin,
		WebAuthnHandle: r.User.WebAuthnHandle,
		CreatedAt:      now.UTC().Truncate(time.Millisecond),
	}
	if len(u.WebAuthnHandle) == 0 {
		return User{}, errors.New("no WebAuthn user handle")
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO users (username, display_name, email, is_admin, webauthn_handle, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		u.Username, u.DisplayName, nullString(u.Email), admin, u.WebAuthnHandle, fmtTime(now)).Scan(&u.ID)
	if err != nil {
		return User{}, mapErr(err)
	}
	c := r.Credential
	c.UserID = u.ID
	if err := insertCredential(ctx, tx, c, now); err != nil {
		return User{}, err
	}
	return u, nil
}
