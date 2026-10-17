package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CodeFile is a learner's saved version of a stub file.
type CodeFile struct {
	Exercise  string
	File      string
	Content   string
	UpdatedAt time.Time
}

// SaveCode stores a file's content and returns the save time.
func (s *Store) SaveCode(ctx context.Context, userID int64, exercise, file, content string) (time.Time, error) {
	now := s.now().UTC().Truncate(time.Millisecond)
	_, err := s.db.ExecContext(ctx, `INSERT INTO code (user_id, exercise, file, content, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, exercise, file) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		userID, exercise, file, content, fmtTime(now))
	if err != nil {
		return time.Time{}, fmt.Errorf("store: save code: %w", mapErr(err))
	}
	return now, nil
}

// Code returns the user's saved files of an exercise by file name. Files
// the user never saved are absent; the caller shows the stub for them.
func (s *Store) Code(ctx context.Context, userID int64, exercise string) (map[string]CodeFile, error) {
	files, err := queryAll(ctx, s.db, func(r scanner) (CodeFile, error) {
		var f CodeFile
		return f, r.Scan(&f.Exercise, &f.File, &f.Content, tcol(&f.UpdatedAt))
	}, `SELECT exercise, file, content, updated_at FROM code WHERE user_id = ? AND exercise = ?`, userID, exercise)
	if err != nil {
		return nil, fmt.Errorf("store: code: %w", err)
	}
	out := make(map[string]CodeFile, len(files))
	for _, f := range files {
		out[f.File] = f
	}
	return out, nil
}

// DeleteCode drops a saved file, so the stub shows again ("Reset file").
func (s *Store) DeleteCode(ctx context.Context, userID int64, exercise, file string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM code WHERE user_id = ? AND exercise = ? AND file = ?`,
		userID, exercise, file); err != nil {
		return fmt.Errorf("store: delete code: %w", err)
	}
	return nil
}

// Status of an exercise for a user.
type Status string

// Statuses. An exercise without a progress row is not started.
const (
	StatusStarted Status = "started"
	StatusPassed  Status = "passed"
)

// Progress is a user's state on 1 exercise.
type Progress struct {
	UserID        int64
	Exercise      string
	Status        Status
	StartedAt     time.Time
	PassedAt      time.Time // zero until passed
	ActiveSeconds int64
	Runs          int
	LastRunAt     time.Time       // zero before the first run
	LastResult    json.RawMessage // the caller's summary of the last run; nil before the first run
	Notes         string
}

// Passed reports whether the exercise is passed.
func (p Progress) Passed() bool { return p.Status == StatusPassed }

const progressCols = `user_id, exercise, status, started_at, passed_at, active_seconds, runs, last_run_at, last_result, notes`

func scanProgress(r scanner) (Progress, error) {
	var p Progress
	var last sql.NullString
	err := r.Scan(&p.UserID, &p.Exercise, &p.Status, tcol(&p.StartedAt), tcol(&p.PassedAt), &p.ActiveSeconds,
		&p.Runs, tcol(&p.LastRunAt), &last, &p.Notes)
	if last.Valid {
		p.LastResult = json.RawMessage(last.String)
	}
	return p, mapErr(err)
}

// ensureProgress is the insert that every progress update starts with: an
// exercise is started by the first thing that happens on it.
const ensureProgress = `INSERT INTO progress (user_id, exercise, status, started_at) VALUES (?, ?, 'started', ?)
	ON CONFLICT (user_id, exercise) DO NOTHING`

// StartExercise marks the exercise started if it is not, and returns its
// progress.
func (s *Store) StartExercise(ctx context.Context, userID int64, exercise string) (Progress, error) {
	var p Progress
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, ensureProgress, userID, exercise, s.stamp()); err != nil {
			return mapErr(err)
		}
		var err error
		p, err = scanProgress(tx.QueryRowContext(ctx, `SELECT `+progressCols+` FROM progress
			WHERE user_id = ? AND exercise = ?`, userID, exercise))
		return err
	})
	if err != nil {
		return Progress{}, fmt.Errorf("store: start exercise: %w", err)
	}
	return p, nil
}

// Progress returns the user's progress on an exercise; ErrNotFound if the
// user never opened it.
func (s *Store) Progress(ctx context.Context, userID int64, exercise string) (Progress, error) {
	p, err := scanProgress(s.db.QueryRowContext(ctx, `SELECT `+progressCols+` FROM progress
		WHERE user_id = ? AND exercise = ?`, userID, exercise))
	if err != nil {
		return Progress{}, fmt.Errorf("store: progress: %w", err)
	}
	return p, nil
}

// UserProgress returns the user's progress on every exercise they opened,
// by exercise slug order.
func (s *Store) UserProgress(ctx context.Context, userID int64) ([]Progress, error) {
	out, err := queryAll(ctx, s.db, scanProgress, `SELECT `+progressCols+` FROM progress WHERE user_id = ? ORDER BY exercise`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: user progress: %w", err)
	}
	return out, nil
}

// progressUpdate starts the exercise if needed and applies 1 update.
func (s *Store) progressUpdate(ctx context.Context, what string, userID int64, exercise, set string, args ...any) error {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, ensureProgress, userID, exercise, s.stamp()); err != nil {
			return mapErr(err)
		}
		_, err := tx.ExecContext(ctx, `UPDATE progress SET `+set+` WHERE user_id = ? AND exercise = ?`,
			append(args, userID, exercise)...)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: %s: %w", what, err)
	}
	return nil
}

// AddActiveTime adds d (rounded down to seconds) to the active time. The
// caller decides what counts as active (a heartbeat while the tab is
// visible and the user typed recently) and caps d.
func (s *Store) AddActiveTime(ctx context.Context, userID int64, exercise string, d time.Duration) error {
	if d < 0 {
		return errors.New("store: add active time: negative duration")
	}
	return s.progressUpdate(ctx, "add active time", userID, exercise,
		`active_seconds = active_seconds + ?`, int64(d/time.Second))
}

// RecordRun counts a test run and keeps its summary.
func (s *Store) RecordRun(ctx context.Context, userID int64, exercise string, result json.RawMessage) error {
	var res any
	if result != nil {
		if !json.Valid(result) {
			return errors.New("store: record run: result is not JSON")
		}
		res = string(result)
	}
	return s.progressUpdate(ctx, "record run", userID, exercise,
		`runs = runs + 1, last_run_at = ?, last_result = ?`, s.stamp(), res)
}

// MarkPassed marks the exercise passed and reports whether this call did
// it. Only the first pass counts: passed_at never moves, so a later run
// cannot change the date on a certificate.
func (s *Store) MarkPassed(ctx context.Context, userID int64, exercise string) (bool, error) {
	var first bool
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.stamp()
		if _, err := tx.ExecContext(ctx, ensureProgress, userID, exercise, now); err != nil {
			return mapErr(err)
		}
		res, err := tx.ExecContext(ctx, `UPDATE progress SET status = 'passed', passed_at = ?
			WHERE user_id = ? AND exercise = ? AND passed_at IS NULL`, now, userID, exercise)
		if err != nil {
			return err
		}
		n, err := rowsAffected(res)
		first = n == 1
		return err
	})
	if err != nil {
		return false, fmt.Errorf("store: mark passed: %w", err)
	}
	return first, nil
}

// SetNotes stores the user's notes on an exercise.
func (s *Store) SetNotes(ctx context.Context, userID int64, exercise, notes string) error {
	return s.progressUpdate(ctx, "set notes", userID, exercise, `notes = ?`, notes)
}

// PassedExercises returns the slugs the user passed, sorted. Certificate
// eligibility starts from it.
func (s *Store) PassedExercises(ctx context.Context, userID int64) ([]string, error) {
	out, err := queryAll(ctx, s.db, func(r scanner) (string, error) {
		var e string
		return e, r.Scan(&e)
	}, `SELECT exercise FROM progress WHERE user_id = ? AND status = 'passed' ORDER BY exercise`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: passed exercises: %w", err)
	}
	return out, nil
}

// Reveal is a revealed level of an exercise's hint ladder (1 to 6: hint 1
// to 3, explanation, walkthrough, solution).
type Reveal struct {
	UserID     int64
	Exercise   string
	Level      int
	RevealedAt time.Time
}

// MaxRevealLevel is the solution, the last level of the ladder.
const MaxRevealLevel = 6

// RecordReveal records that the user revealed level. Level 1 is always
// allowed; level N needs N-1. The check and the insert are 1 statement, so
// 2 concurrent requests cannot skip a level. Revealing a level again is
// not an error: it returns the first reveal and created false. A skipped
// level gives ErrRevealOrder. The soft time gates are the caller's job.
func (s *Store) RecordReveal(ctx context.Context, userID int64, exercise string, level int) (Reveal, bool, error) {
	if level < 1 || level > MaxRevealLevel {
		return Reveal{}, false, fmt.Errorf("store: record reveal: level %d out of range", level)
	}
	var r Reveal
	var created bool
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.stamp()
		res, err := tx.ExecContext(ctx, `INSERT INTO reveals (user_id, exercise, level, revealed_at)
			SELECT ?, ?, ?, ?
			WHERE ? = 1 OR EXISTS (SELECT 1 FROM reveals WHERE user_id = ? AND exercise = ? AND level = ?)
			ON CONFLICT (user_id, exercise, level) DO NOTHING`,
			userID, exercise, level, now, level, userID, exercise, level-1)
		if err != nil {
			return mapErr(err)
		}
		n, err := rowsAffected(res)
		if err != nil {
			return err
		}
		created = n == 1
		r, err = scanReveal(tx.QueryRowContext(ctx, `SELECT user_id, exercise, level, revealed_at FROM reveals
			WHERE user_id = ? AND exercise = ? AND level = ?`, userID, exercise, level))
		if errors.Is(err, ErrNotFound) {
			return ErrRevealOrder
		}
		return err
	})
	if err != nil {
		return Reveal{}, false, fmt.Errorf("store: record reveal: %w", err)
	}
	return r, created, nil
}

// Reveals returns the user's revealed levels of an exercise, in level
// order.
func (s *Store) Reveals(ctx context.Context, userID int64, exercise string) ([]Reveal, error) {
	return s.reveals(ctx, `WHERE user_id = ? AND exercise = ? ORDER BY level`, userID, exercise)
}

// UserReveals returns every level the user revealed, by exercise and
// level. Dashboard and certificate statistics count them.
func (s *Store) UserReveals(ctx context.Context, userID int64) ([]Reveal, error) {
	return s.reveals(ctx, `WHERE user_id = ? ORDER BY exercise, level`, userID)
}

func (s *Store) reveals(ctx context.Context, where string, args ...any) ([]Reveal, error) {
	out, err := queryAll(ctx, s.db, scanReveal, `SELECT user_id, exercise, level, revealed_at FROM reveals `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: reveals: %w", err)
	}
	return out, nil
}

func scanReveal(r scanner) (Reveal, error) {
	var rv Reveal
	err := r.Scan(&rv.UserID, &rv.Exercise, &rv.Level, tcol(&rv.RevealedAt))
	return rv, mapErr(err)
}

// UserSummary is a row of the admin's user list.
type UserSummary struct {
	User
	Started       int // exercises opened, passed included
	Passed        int
	ActiveSeconds int64
	Runs          int
	Reveals       int
	LastActive    time.Time // the last run, or zero
}

// UserSummaries returns every user with totals of their progress.
func (s *Store) UserSummaries(ctx context.Context) ([]UserSummary, error) {
	out, err := queryAll(ctx, s.db, func(r scanner) (UserSummary, error) {
		var us UserSummary
		u := &us.User
		err := r.Scan(&u.ID, &u.Username, &u.DisplayName, scol(&u.Email), tcol(&u.EmailVerifiedAt),
			&u.IsAdmin, tcol(&u.DisabledAt), &u.WebAuthnHandle, tcol(&u.CreatedAt),
			&us.Started, &us.Passed, &us.ActiveSeconds, &us.Runs, tcol(&us.LastActive), &us.Reveals)
		return us, err
	}, `SELECT `+prefixed("u.", userCols)+`,
		COUNT(p.exercise), COALESCE(SUM(p.status = 'passed'), 0), COALESCE(SUM(p.active_seconds), 0),
		COALESCE(SUM(p.runs), 0), MAX(p.last_run_at),
		(SELECT COUNT(*) FROM reveals r WHERE r.user_id = u.id)
		FROM users u LEFT JOIN progress p ON p.user_id = u.id
		GROUP BY u.id ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("store: user summaries: %w", err)
	}
	return out, nil
}
