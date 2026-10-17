package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Credential is a passkey. The fields mirror what go-webauthn needs to
// verify a later login, plus a label and timestamps for the account page.
type Credential struct {
	ID              []byte
	UserID          int64
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	CloneWarning    bool
	Transports      []string // "usb", "nfc", "ble", "internal", "hybrid"
	Attachment      string   // "platform" or "cross-platform"; may be ""
	UserPresent     bool
	UserVerified    bool
	BackupEligible  bool
	BackupState     bool
	Name            string // the user's label, for example "MacBook"
	CreatedAt       time.Time
	LastUsedAt      time.Time // zero until the first login with it
}

const credentialCols = `id, user_id, public_key, attestation_type, aaguid, sign_count, clone_warning, transports,
	attachment, user_present, user_verified, backup_eligible, backup_state, name, created_at, last_used_at`

func scanCredential(r scanner) (Credential, error) {
	var c Credential
	var transports string
	var signCount int64
	err := r.Scan(&c.ID, &c.UserID, &c.PublicKey, &c.AttestationType, &c.AAGUID, &signCount, &c.CloneWarning,
		&transports, &c.Attachment, &c.UserPresent, &c.UserVerified, &c.BackupEligible, &c.BackupState,
		&c.Name, tcol(&c.CreatedAt), tcol(&c.LastUsedAt))
	if err != nil {
		return Credential{}, mapErr(err)
	}
	c.SignCount = uint32(signCount) //nolint:gosec // stored from a uint32
	if transports != "" {
		c.Transports = strings.Split(transports, ",")
	}
	return c, nil
}

func insertCredential(ctx context.Context, tx *sql.Tx, c Credential, now time.Time) error {
	if len(c.ID) == 0 || len(c.PublicKey) == 0 {
		return errors.New("credential without ID or public key")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO credentials (id, user_id, public_key, attestation_type, aaguid,
		sign_count, clone_warning, transports, attachment, user_present, user_verified, backup_eligible,
		backup_state, name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.PublicKey, c.AttestationType, c.AAGUID, int64(c.SignCount), c.CloneWarning,
		strings.Join(c.Transports, ","), c.Attachment, c.UserPresent, c.UserVerified, c.BackupEligible,
		c.BackupState, c.Name, fmtTime(now))
	return mapErr(err)
}

// AddCredential stores a new passkey for c.UserID.
func (s *Store) AddCredential(ctx context.Context, c Credential) error {
	err := s.tx(ctx, func(tx *sql.Tx) error { return insertCredential(ctx, tx, c, s.now()) })
	if err != nil {
		return fmt.Errorf("store: add credential: %w", err)
	}
	return nil
}

// Credential returns the passkey with the given credential ID.
func (s *Store) Credential(ctx context.Context, id []byte) (Credential, error) {
	c, err := scanCredential(s.db.QueryRowContext(ctx, `SELECT `+credentialCols+` FROM credentials WHERE id = ?`, id))
	if err != nil {
		return Credential{}, fmt.Errorf("store: credential: %w", err)
	}
	return c, nil
}

// Credentials returns a user's passkeys, oldest first.
func (s *Store) Credentials(ctx context.Context, userID int64) ([]Credential, error) {
	out, err := queryAll(ctx, s.db, scanCredential, `SELECT `+credentialCols+` FROM credentials WHERE user_id = ?
		ORDER BY created_at, rowid`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: credentials: %w", err)
	}
	return out, nil
}

// CredentialUse is what a successful login changes on a passkey.
type CredentialUse struct {
	SignCount    uint32
	CloneWarning bool
	BackupState  bool
	UserVerified bool
}

// CredentialUsed records a login with the passkey.
func (s *Store) CredentialUsed(ctx context.Context, id []byte, u CredentialUse) error {
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET sign_count = ?, clone_warning = ?, backup_state = ?,
		user_verified = ?, last_used_at = ? WHERE id = ?`,
		int64(u.SignCount), u.CloneWarning, u.BackupState, u.UserVerified, s.stamp(), id)
	if err := found(res, err); err != nil {
		return fmt.Errorf("store: credential used: %w", err)
	}
	return nil
}

// RenameCredential sets the label of 1 of the user's passkeys.
func (s *Store) RenameCredential(ctx context.Context, userID int64, id []byte, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	if err := found(res, err); err != nil {
		return fmt.Errorf("store: rename credential: %w", err)
	}
	return nil
}

// DeleteCredential removes 1 of the user's passkeys. Removing the last one
// gives ErrLastCredential: the account would be locked out. The check and
// the delete are 1 statement, so 2 concurrent deletes cannot both pass it.
func (s *Store) DeleteCredential(ctx context.Context, userID int64, id []byte) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ? AND user_id = ?
		AND (SELECT COUNT(*) FROM credentials WHERE user_id = ?) > 1`, id, userID, userID)
	if err != nil {
		return fmt.Errorf("store: delete credential: %w", err)
	}
	n, err := rowsAffected(res)
	if err != nil {
		return fmt.Errorf("store: delete credential: %w", err)
	}
	if n == 1 {
		return nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM credentials WHERE id = ? AND user_id = ?)`,
		id, userID).Scan(&exists); err != nil {
		return fmt.Errorf("store: delete credential: %w", err)
	}
	if exists {
		return fmt.Errorf("store: delete credential: %w", ErrLastCredential)
	}
	return fmt.Errorf("store: delete credential: %w", ErrNotFound)
}

// found turns an update or delete result that matched no row into
// ErrNotFound.
func found(res sql.Result, err error) error {
	if err != nil {
		return mapErr(err)
	}
	n, err := rowsAffected(res)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
