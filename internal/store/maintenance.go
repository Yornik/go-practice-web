package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Probe proves that the database can be written and read: it writes a
// random value to the probe table and reads it back. Readiness uses it
// because a volume that went read-only still answers reads; a SELECT 1
// check would keep a pod ready that cannot save anyone's code.
func (s *Store) Probe(ctx context.Context) error {
	want := rand.Text()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO probe (id, value, updated_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, want, s.stamp()); err != nil {
		return fmt.Errorf("store: probe write: %w", err)
	}
	var got string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM probe WHERE id = 1`).Scan(&got); err != nil {
		return fmt.Errorf("store: probe read: %w", err)
	}
	if got != want {
		return fmt.Errorf("store: probe read %q after writing %q", got, want)
	}
	return nil
}

// BackupName returns the backup file name for the store's current weekday,
// for example "go-practice-monday.db". 7 names rotate, so the directory
// holds at most a week of backups.
func (s *Store) BackupName() string {
	return "go-practice-" + strings.ToLower(s.now().Weekday().String()) + ".db"
}

// Backup writes a consistent copy of the database to dir with VACUUM INTO,
// checks the copy with PRAGMA integrity_check, and then renames it over the
// weekday's file. A failed or corrupt copy never replaces a good backup. It
// returns the path of the backup.
func (s *Store) Backup(ctx context.Context, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("store: backup: %w", err)
	}
	final := filepath.Join(dir, s.BackupName())
	tmp := final + ".tmp"
	// VACUUM INTO refuses to overwrite; a leftover is from a crashed backup.
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("store: backup: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("store: backup: vacuum into %s: %w", tmp, err)
	}
	if err := checkIntegrity(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("store: backup: %w", err)
	}
	// VACUUM INTO does not fsync the copy; the rename must not publish a
	// file whose data is still in the page cache.
	if err := syncPath(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("store: backup: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("store: backup: %w", err)
	}
	if err := syncPath(dir); err != nil {
		return "", fmt.Errorf("store: backup: %w", err)
	}
	return final, nil
}

// checkIntegrity opens a database file read only and runs PRAGMA
// integrity_check on it.
func checkIntegrity(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path+"?_query_only=1")
	if err != nil {
		return fmt.Errorf("integrity check of %s: %w", path, err)
	}
	defer func() { _ = db.Close() }() // read only: nothing to lose on close
	lines, err := queryAll(ctx, db, func(r scanner) (string, error) {
		var line string
		return line, r.Scan(&line)
	}, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("integrity check of %s: %w", path, err)
	}
	if len(lines) != 1 || lines[0] != "ok" {
		return fmt.Errorf("integrity check of %s: %s", path, strings.Join(lines, "; "))
	}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table'`).Scan(&tables); err != nil {
		return fmt.Errorf("integrity check of %s: %w", path, err)
	}
	if tables == 0 {
		return fmt.Errorf("integrity check of %s: no tables", path)
	}
	return nil
}

func syncPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	return f.Close()
}

// prefixed qualifies every column of a column list, for joins.
func prefixed(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
