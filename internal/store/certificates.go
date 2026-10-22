package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Certificate is an issued certificate. Payload is the exact canonical JSON
// that Signature signs; the other fields repeat parts of it for queries.
type Certificate struct {
	ID         string // GP-XXXX-XXXX-XXXX
	UserID     int64
	Kind       string // tier1 to tier10, or course
	NameOnCert string
	IssuedAt   time.Time
	ShowStats  bool
	Stats      json.RawMessage // nil when ShowStats is false
	Payload    []byte
	Signature  []byte
	RevokedAt  time.Time // zero while valid
	ReplacedBy string    // the re-issued certificate's ID, when revoked by a re-issue
}

// Revoked reports whether the certificate was revoked.
func (c Certificate) Revoked() bool { return !c.RevokedAt.IsZero() }

const certificateCols = `id, user_id, kind, name_on_cert, issued_at, show_stats, stats, payload, signature, revoked_at, replaced_by`

func scanCertificate(r scanner) (Certificate, error) {
	var c Certificate
	var stats sql.NullString
	var payload string
	err := r.Scan(&c.ID, &c.UserID, &c.Kind, &c.NameOnCert, tcol(&c.IssuedAt), &c.ShowStats, &stats, &payload,
		&c.Signature, tcol(&c.RevokedAt), scol(&c.ReplacedBy))
	if stats.Valid {
		c.Stats = json.RawMessage(stats.String)
	}
	c.Payload = []byte(payload)
	return c, mapErr(err)
}

// IssueCertificate stores a signed certificate. If the user holds a valid
// certificate of the same kind, it is revoked and points to the new one, in
// the same transaction; the old ID is returned ("" when there was none). A
// colliding ID gives a *ConflictError; the caller draws a new ID and signs
// again.
func (s *Store) IssueCertificate(ctx context.Context, c Certificate) (string, error) {
	if c.ID == "" || len(c.Payload) == 0 || len(c.Signature) == 0 || c.IssuedAt.IsZero() {
		return "", errors.New("store: issue certificate: ID, payload, signature and issue time are required")
	}
	if !json.Valid(c.Payload) || c.Stats != nil && !json.Valid(c.Stats) {
		return "", errors.New("store: issue certificate: payload or stats is not JSON")
	}
	var old string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.stamp()
		err := tx.QueryRowContext(ctx, `UPDATE certificates SET revoked_at = ?
			WHERE user_id = ? AND kind = ? AND revoked_at IS NULL RETURNING id`, now, c.UserID, c.Kind).Scan(&old)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var stats any
		if c.Stats != nil {
			stats = string(c.Stats)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at,
			show_stats, stats, payload, signature) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.UserID, c.Kind, c.NameOnCert, fmtTime(c.IssuedAt), c.ShowStats, stats, string(c.Payload),
			c.Signature); err != nil {
			return mapErr(err)
		}
		if old != "" {
			_, err := tx.ExecContext(ctx, `UPDATE certificates SET replaced_by = ? WHERE id = ?`, c.ID, old)
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("store: issue certificate: %w", err)
	}
	return old, nil
}

// Certificate returns a certificate, valid or revoked.
func (s *Store) Certificate(ctx context.Context, id string) (Certificate, error) {
	c, err := scanCertificate(s.db.QueryRowContext(ctx, `SELECT `+certificateCols+` FROM certificates WHERE id = ?`,
		strings.ToUpper(strings.TrimSpace(id))))
	if err != nil {
		return Certificate{}, fmt.Errorf("store: certificate: %w", err)
	}
	return c, nil
}

// UserCertificates returns the user's valid certificates, oldest first.
func (s *Store) UserCertificates(ctx context.Context, userID int64) ([]Certificate, error) {
	out, err := queryAll(ctx, s.db, scanCertificate, `SELECT `+certificateCols+` FROM certificates
		WHERE user_id = ? AND revoked_at IS NULL ORDER BY issued_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: user certificates: %w", err)
	}
	return out, nil
}

// FirstIssued returns, per kind of the user's valid certificates, when the
// first certificate of its chain of re-issues was issued. A re-issue fixes
// a name or the activity row; it is the same tag, stamped on the first
// date. A claim that replaces a certificate of an earlier course goes on
// with its chain too. A revoke without a replacement ends a chain, so a
// claim after it starts a new one.
func (s *Store) FirstIssued(ctx context.Context, userID int64) (map[string]time.Time, error) {
	type row struct {
		kind string
		at   time.Time
	}
	rows, err := queryAll(ctx, s.db, func(r scanner) (row, error) {
		var v row
		err := r.Scan(&v.kind, tcol(&v.at))
		return v, err
	}, `WITH RECURSIVE chain (kind, id, issued_at) AS (
			SELECT kind, id, issued_at FROM certificates WHERE user_id = ? AND revoked_at IS NULL
			UNION ALL
			SELECT chain.kind, c.id, c.issued_at FROM certificates c JOIN chain ON c.replaced_by = chain.id
			WHERE c.user_id = ?
		)
		SELECT kind, MIN(issued_at) FROM chain GROUP BY kind`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: first issued: %w", err)
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.kind] = r.at
	}
	return out, nil
}

// RevokeCertificate revokes a valid certificate without a replacement.
func (s *Store) RevokeCertificate(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE certificates SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		s.stamp(), id)
	if err := found(res, err); err != nil {
		return fmt.Errorf("store: revoke certificate: %w", err)
	}
	return nil
}

// Setting returns a setting's value; ErrNotFound if it is not set.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return "", fmt.Errorf("store: setting %q: %w", key, mapErr(err))
	}
	return v, nil
}

// SetSetting stores a setting, replacing its value.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}

// SetSettingIfAbsent stores value under key unless the key is set, and
// returns the value that is stored afterwards. 2 processes that start at
// once both end up with the same value, which is what a generated signing
// key needs.
func (s *Store) SetSettingIfAbsent(ctx context.Context, key, value string) (string, error) {
	var v string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING`,
			key, value); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	})
	if err != nil {
		return "", fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return v, nil
}

// AuditEntry is a security-relevant event: a sign-in, a passkey change, a
// recovery, an admin action.
type AuditEntry struct {
	ID        int64
	UserID    int64  // 0 when no user is known (a failed login, a recovery request)
	Event     string // a short dotted name, for example "passkey.added"
	Detail    json.RawMessage
	IP        string
	CreatedAt time.Time
}

// Audit appends an entry. detail is marshalled to JSON; nil gives {}.
func (s *Store) Audit(ctx context.Context, userID int64, event string, detail any, ip string) error {
	d := []byte("{}")
	if detail != nil {
		var err error
		if d, err = json.Marshal(detail); err != nil {
			return fmt.Errorf("store: audit %s: %w", event, err)
		}
	}
	var user any
	if userID != 0 {
		user = userID
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO audit (user_id, event, detail, ip, created_at) VALUES (?, ?, ?, ?, ?)`,
		user, event, string(d), ip, s.stamp()); err != nil {
		return fmt.Errorf("store: audit %s: %w", event, mapErr(err))
	}
	return nil
}

// AuditQuery selects audit entries, newest first.
type AuditQuery struct {
	UserID   int64 // 0 for every user
	BeforeID int64 // 0 for the newest; pass the last ID of a page for the next
	Limit    int   // 0 for 100
}

// AuditLog returns audit entries.
func (s *Store) AuditLog(ctx context.Context, q AuditQuery) ([]AuditEntry, error) {
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 100
	}
	where, args := []string{"1 = 1"}, []any{}
	if q.UserID != 0 {
		where, args = append(where, "user_id = ?"), append(args, q.UserID)
	}
	if q.BeforeID != 0 {
		where, args = append(where, "id < ?"), append(args, q.BeforeID)
	}
	out, err := queryAll(ctx, s.db, func(r scanner) (AuditEntry, error) {
		var e AuditEntry
		var user sql.NullInt64
		var detail string
		err := r.Scan(&e.ID, &user, &e.Event, &detail, &e.IP, tcol(&e.CreatedAt))
		e.UserID, e.Detail = user.Int64, json.RawMessage(detail)
		return e, err
	}, `SELECT id, user_id, event, detail, ip, created_at FROM audit WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, append(args, q.Limit)...)
	if err != nil {
		return nil, fmt.Errorf("store: audit log: %w", err)
	}
	return out, nil
}

// Counts are the totals the metrics endpoint exports.
type Counts struct {
	Users          int
	ActiveSessions int
	Passed         int // passed exercises over all users
	Certificates   int // valid certificates
}

// Counts returns the totals in 1 query.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM sessions WHERE expires_at > ?),
		(SELECT COUNT(*) FROM progress WHERE status = 'passed'),
		(SELECT COUNT(*) FROM certificates WHERE revoked_at IS NULL)`, s.stamp()).
		Scan(&c.Users, &c.ActiveSessions, &c.Passed, &c.Certificates)
	if err != nil {
		return Counts{}, fmt.Errorf("store: counts: %w", err)
	}
	return c, nil
}
