// Package store is the SQLite database of the web server: users, passkeys,
// sessions, tokens, code, progress, hint reveals, certificates, settings and
// the audit log. Every method is safe for concurrent use. Methods that must
// be atomic (using a token, marking an exercise passed, revealing a level,
// issuing a certificate) are, so callers need no locks of their own.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Errors that callers branch on. Errors wrap them with context; test with
// errors.Is.
var (
	ErrNotFound       = errors.New("store: not found")
	ErrConflict       = errors.New("store: already exists")
	ErrTokenInvalid   = errors.New("store: token unknown, used or expired")
	ErrLastCredential = errors.New("store: cannot remove the last passkey")
	ErrRevealOrder    = errors.New("store: the previous level is not revealed")
)

// ConflictError reports which unique column a write collided with, so the
// UI can say "username taken" rather than "conflict". It matches
// ErrConflict with errors.Is.
type ConflictError struct {
	Column string // for example "users.username"
}

func (e *ConflictError) Error() string { return "store: " + e.Column + " already exists" }

// Is makes errors.Is(err, ErrConflict) true.
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// Store is the database. Create it with Open.
type Store struct {
	db   *sql.DB
	path string
	now  func() time.Time
}

// Option configures Open.
type Option func(*Store)

// WithClock sets the clock for every timestamp and expiry check. Tests use
// it; the default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Store) { s.now = now }
}

// Pool limits. SQLite with a rollback journal has 1 writer at a time, so
// more connections only help concurrent reads; a few are plenty for 1
// replica with a handful of learners.
const (
	maxOpenConns = 4
	busyTimeout  = 10 * time.Second
)

// Open opens (or creates) the database file at path and applies the
// migrations. The file must be on a filesystem with working POSIX locks.
func Open(ctx context.Context, path string, opts ...Option) (*Store, error) {
	if path == "" || strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("store: bad database path %q", path)
	}
	s := &Store{path: path, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	// Connections never go stale for a local file; keeping them saves
	// re-running the pragmas.
	db.SetConnMaxLifetime(0)
	s.db = db
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// dsn builds the modernc.org/sqlite data source name. The pragmas run on
// every new connection, which matters for foreign_keys and synchronous:
// they are per connection, not per file.
func dsn(path string, readOnly bool) string {
	q := url.Values{}
	q.Set("_busy_timeout", strconv.FormatInt(busyTimeout.Milliseconds(), 10))
	q.Set("_foreign_keys", "1")
	// DELETE, not WAL: the data volume is NFS, and WAL needs shared memory
	// that NFS does not provide.
	q.Set("_journal_mode", "DELETE")
	// FULL syncs the journal before and after each commit; a power loss on
	// the NFS server must not lose a passkey.
	q.Set("_synchronous", "FULL")
	// BEGIN IMMEDIATE takes the write lock at the start of a transaction. A
	// deferred transaction that reads and then writes can fail with
	// SQLITE_BUSY at once, without the busy timeout, when 2 of them race.
	q.Set("_txlock", "immediate")
	if readOnly {
		q.Set("_query_only", "1")
	}
	return path + "?" + q.Encode()
}

// Close closes the database.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// migrate applies the embedded migrations that the database lacks, each in
// its own transaction together with its row in schema_migrations.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("store: list migrations: %w", err)
	}
	type migration struct {
		version int
		name    string
		file    string
	}
	var ms []migration
	for _, f := range files {
		base := path.Base(f)
		num, name, ok := strings.Cut(strings.TrimSuffix(base, ".sql"), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || len(num) != 4 {
			return fmt.Errorf("store: migration %s is not NNNN_name.sql", base)
		}
		ms = append(ms, migration{version: v, name: name, file: f})
	}
	slices.SortFunc(ms, func(a, b migration) int { return a.version - b.version })

	versions, err := queryAll(ctx, s.db, func(r scanner) (int, error) {
		var v int
		return v, r.Scan(&v)
	}, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("store: read schema_migrations: %w", err)
	}
	applied := map[int]bool{}
	for _, v := range versions {
		applied[v] = true
	}
	for v := range applied {
		if !slices.ContainsFunc(ms, func(m migration) bool { return m.version == v }) {
			// An older binary must not run against a schema it does not know.
			return fmt.Errorf("store: database has migration %04d, which this build does not know", v)
		}
	}

	backedUp := false
	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if len(applied) > 0 && !backedUp {
			// A migration is one way: the previous build refuses the new
			// schema, and the daily backup can be a day old. A copy from
			// right before the first pending migration makes a rollback
			// lose nothing; without it the start stops.
			if _, err := s.backupTo(ctx, s.BackupDir(), fmt.Sprintf("go-practice-before-%04d.db", m.version)); err != nil {
				return fmt.Errorf("store: before migration %04d_%s: %w", m.version, m.name, err)
			}
			backedUp = true
		}
		body, err := migrations.ReadFile(m.file)
		if err != nil {
			return fmt.Errorf("store: read %s: %w", m.file, err)
		}
		err = s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
				m.version, m.name, s.stamp())
			return err
		})
		if err != nil {
			return fmt.Errorf("store: migration %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("store: schema version: %w", err)
	}
	return int(v.Int64), nil
}

// tx runs fn in a transaction (BEGIN IMMEDIATE, see dsn) and commits when
// fn returns nil.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// timeLayout is RFC 3339 in UTC with a fixed width, so that comparing the
// text in SQL compares the times.
const timeLayout = "2006-01-02T15:04:05.000Z"

// stamp returns the current time as stored.
func (s *Store) stamp() string { return fmtTime(s.now()) }

func fmtTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad time %q in database: %w", s, err)
	}
	return t.UTC(), nil
}

// timeCol scans a TEXT time column into *time.Time. NULL gives the zero
// time.
type timeCol struct{ t *time.Time }

func (c timeCol) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*c.t = time.Time{}
		return nil
	case string:
		t, err := parseTime(v)
		*c.t = t
		return err
	case []byte:
		t, err := parseTime(string(v))
		*c.t = t
		return err
	}
	return fmt.Errorf("cannot scan %T into a time", src)
}

func tcol(t *time.Time) timeCol { return timeCol{t} }

// nullString maps "" to NULL for nullable text columns.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// strCol scans nullable TEXT into a string; NULL gives "".
type strCol struct{ s *string }

func (c strCol) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*c.s = ""
	case string:
		*c.s = v
	case []byte:
		*c.s = string(v)
	default:
		return fmt.Errorf("cannot scan %T into a string", src)
	}
	return nil
}

func scol(s *string) strCol { return strCol{s} }

// mapErr turns driver errors into the package's errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			// The message ends in "UNIQUE constraint failed: users.username
			// (2067)"; a composite key lists "t.a, t.b".
			col := se.Error()
			if i := strings.LastIndex(col, "failed: "); i >= 0 {
				col = col[i+len("failed: "):]
			}
			if i := strings.LastIndex(col, " ("); i >= 0 {
				col = col[:i]
			}
			return &ConflictError{Column: col}
		}
	}
	return err
}

func rowsAffected(res sql.Result) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// queryAll runs a query and scans every row with scan.
func queryAll[T any](ctx context.Context, db *sql.DB, scan func(scanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	// After a full iteration, Close has nothing to report that Err does not.
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
