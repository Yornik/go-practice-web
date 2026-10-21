package store

import (
	"context"
	"database/sql"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
)

// openAtVersion creates a database with the migrations up to version v
// applied, as an older build left it, and runs the seed statements on it.
func openAtVersion(t *testing.T, v int, seed ...string) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	files := []string{"0001_init.sql", "0002_invite_mailed.sql"}[:v]
	for i, f := range files {
		body, err := migrations.ReadFile("migrations/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		name := strings.TrimSuffix(f[5:], ".sql")
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?, '2026-10-01T00:00:00.000Z')`, i+1, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range seed {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%.60s: %v", q, err)
		}
	}
	return path
}

// The rows of a learner of the first course: alice passed 01-logfmt,
// started 25-loadbalancer and revealed 2 hints of 07-retry; she claimed the
// tier 1 certificate twice (the first is revoked and points to the second)
// and the course certificate. bob holds tier 2 and a row for an exercise
// that the rename table does not name.
var firstCourseRows = []string{
	`INSERT INTO users (id, username, display_name, email, webauthn_handle, created_at) VALUES
		(1, 'alice', 'Alice', 'alice@example.com', x'01', '2026-09-01T08:00:00.000Z'),
		(2, 'bob', 'Bob', 'bob@example.com', x'02', '2026-09-02T08:00:00.000Z')`,
	`INSERT INTO code (user_id, exercise, file, content, updated_at) VALUES
		(1, '01-logfmt', 'logfmt.go', 'package logfmt // alice', '2026-09-03T10:00:00.000Z'),
		(1, '07-retry', 'retry.go', 'package retry', '2026-09-04T10:00:00.000Z'),
		(1, '07-retry', 'retryafter.go', 'package retry // after', '2026-09-04T10:01:00.000Z'),
		(2, '99-scratch', 'x.go', 'package x', '2026-09-05T10:00:00.000Z')`,
	`INSERT INTO progress (user_id, exercise, status, started_at, passed_at, active_seconds, runs, last_run_at, last_result, notes) VALUES
		(1, '01-logfmt', 'passed', '2026-09-03T09:00:00.000Z', '2026-09-03T11:00:00.000Z', 5400, 12, '2026-09-03T11:00:00.000Z', '{"passed":9}', 'quoted values'),
		(1, '25-loadbalancer', 'started', '2026-09-20T09:00:00.000Z', NULL, 60, 0, NULL, NULL, ''),
		(2, '04-grep', 'started', '2026-09-06T09:00:00.000Z', NULL, 30, 1, NULL, NULL, '')`,
	`INSERT INTO reveals (user_id, exercise, level, revealed_at) VALUES
		(1, '07-retry', 1, '2026-09-04T11:00:00.000Z'),
		(1, '07-retry', 2, '2026-09-04T11:20:00.000Z')`,
	`INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, show_stats, stats, payload, signature) VALUES
		('GP-AAAA-AAAA-AAAA', 1, 'tier1', 'Alice', '2026-09-10T09:00:00.000Z', 0, NULL,
		 '{"id":"GP-AAAA-AAAA-AAAA","kind":"tier1","name":"Alice","issuedAt":"2026-09-10T09:00:00Z","exercises":["01-logfmt","02-accesslog","03-latency"]}', x'0a0b0c')`,
	`UPDATE certificates SET revoked_at = '2026-09-11T09:00:00.000Z' WHERE id = 'GP-AAAA-AAAA-AAAA'`,
	`INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, show_stats, stats, payload, signature) VALUES
		('GP-BBBB-BBBB-BBBB', 1, 'tier1', 'Alice A.', '2026-09-11T09:00:00.000Z', 1, '{"runs":12}',
		 '{"id":"GP-BBBB-BBBB-BBBB","kind":"tier1","name":"Alice A.","issuedAt":"2026-09-11T09:00:00Z","exercises":["01-logfmt","02-accesslog","03-latency"]}', x'0d0e0f')`,
	`UPDATE certificates SET replaced_by = 'GP-BBBB-BBBB-BBBB' WHERE id = 'GP-AAAA-AAAA-AAAA'`,
	`INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, show_stats, stats, payload, signature) VALUES
		('GP-CCCC-CCCC-CCCC', 1, 'course', 'Alice', '2026-09-30T09:00:00.000Z', 0, NULL, '{"id":"GP-CCCC-CCCC-CCCC","kind":"course"}', x'10'),
		('GP-DDDD-DDDD-DDDD', 2, 'tier2', 'Bob', '2026-09-12T09:00:00.000Z', 0, NULL, '{"id":"GP-DDDD-DDDD-DDDD","kind":"tier2"}', x'11')`,
}

func TestMigrationCourseV2(t *testing.T) {
	ctx := context.Background()
	path := openAtVersion(t, 2, firstCourseRows...)
	var before []Certificate
	func() {
		db, err := sql.Open("sqlite", dsn(path, true))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if before, err = queryAll(ctx, db, scanCertificate, `SELECT `+certificateCols+` FROM certificates ORDER BY id`); err != nil {
			t.Fatal(err)
		}
	}()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if v, err := s.SchemaVersion(ctx); err != nil || v != 3 {
		t.Fatalf("SchemaVersion = %d, %v", v, err)
	}

	type codeRow struct{ user, exercise, file, content, updated string }
	code, err := queryAll(ctx, s.db, func(r scanner) (codeRow, error) {
		var c codeRow
		return c, r.Scan(&c.user, &c.exercise, &c.file, &c.content, &c.updated)
	}, `SELECT user_id, exercise, file, content, updated_at FROM code ORDER BY user_id, exercise, file`)
	if err != nil {
		t.Fatal(err)
	}
	wantCode := []codeRow{
		{"1", "26-logfmt", "logfmt.go", "package logfmt // alice", "2026-09-03T10:00:00.000Z"},
		{"1", "32-retry", "retry.go", "package retry", "2026-09-04T10:00:00.000Z"},
		{"1", "32-retry", "retryafter.go", "package retry // after", "2026-09-04T10:01:00.000Z"},
		{"2", "99-scratch", "x.go", "package x", "2026-09-05T10:00:00.000Z"},
	}
	if !slices.Equal(code, wantCode) {
		t.Errorf("code =\n%v\nwant\n%v", code, wantCode)
	}

	p, err := s.Progress(ctx, 1, "26-logfmt")
	if err != nil || p.Status != StatusPassed || p.ActiveSeconds != 5400 || p.Runs != 12 || p.Notes != "quoted values" ||
		string(p.LastResult) != `{"passed":9}` || fmtTime(p.PassedAt) != "2026-09-03T11:00:00.000Z" || fmtTime(p.StartedAt) != "2026-09-03T09:00:00.000Z" {
		t.Errorf("progress of 26-logfmt = %+v, %v", p, err)
	}
	passed, err := s.PassedExercises(ctx, 1)
	if err != nil || !slices.Equal(passed, []string{"26-logfmt"}) {
		t.Errorf("PassedExercises = %v, %v", passed, err)
	}
	if p, err := s.Progress(ctx, 1, "50-loadbalancer"); err != nil || p.Status != StatusStarted || p.ActiveSeconds != 60 {
		t.Errorf("progress of 50-loadbalancer = %+v, %v", p, err)
	}
	if p, err := s.Progress(ctx, 2, "29-grep"); err != nil || p.Runs != 1 {
		t.Errorf("bob's progress of 29-grep = %+v, %v", p, err)
	}
	for old := range course.Renames() {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM code WHERE exercise = ?1) +
			(SELECT COUNT(*) FROM progress WHERE exercise = ?1) + (SELECT COUNT(*) FROM reveals WHERE exercise = ?1)`, old).Scan(&n); err != nil || n != 0 {
			t.Errorf("%d rows keep the old slug %s, %v", n, old, err)
		}
	}
	reveals, err := s.UserReveals(ctx, 1)
	if err != nil || len(reveals) != 2 {
		t.Fatalf("UserReveals = %v, %v", reveals, err)
	}
	for i, rv := range reveals {
		if rv.Exercise != "32-retry" || rv.Level != i+1 {
			t.Errorf("reveal %d = %+v", i, rv)
		}
	}

	// Every certificate row is the same but for the kind of a tier.
	wantKind := map[string]string{
		"GP-AAAA-AAAA-AAAA": "tier6", "GP-BBBB-BBBB-BBBB": "tier6",
		"GP-CCCC-CCCC-CCCC": "course", "GP-DDDD-DDDD-DDDD": "tier7",
	}
	after, err := queryAll(ctx, s.db, scanCertificate, `SELECT `+certificateCols+` FROM certificates ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) || len(after) != 4 {
		t.Fatalf("%d certificates before, %d after", len(before), len(after))
	}
	for i, a := range after {
		b := before[i]
		if a.Kind != wantKind[a.ID] {
			t.Errorf("%s: kind %s, want %s", a.ID, a.Kind, wantKind[a.ID])
		}
		b.Kind = a.Kind
		if a.ID != b.ID || a.UserID != b.UserID || a.NameOnCert != b.NameOnCert || !a.IssuedAt.Equal(b.IssuedAt) ||
			a.ShowStats != b.ShowStats || string(a.Stats) != string(b.Stats) || string(a.Payload) != string(b.Payload) ||
			string(a.Signature) != string(b.Signature) || !a.RevokedAt.Equal(b.RevokedAt) || a.ReplacedBy != b.ReplacedBy {
			t.Errorf("certificate changed:\n got %+v\nwant %+v", a, b)
		}
	}
	if old, _ := s.Certificate(ctx, "GP-AAAA-AAAA-AAAA"); !old.Revoked() || old.ReplacedBy != "GP-BBBB-BBBB-BBBB" {
		t.Errorf("the revoked certificate = %+v", old)
	}
	held, err := s.UserCertificates(ctx, 1)
	if err != nil || len(held) != 2 || held[0].ID != "GP-BBBB-BBBB-BBBB" || held[1].ID != "GP-CCCC-CCCC-CCCC" {
		t.Errorf("UserCertificates(alice) = %v, %v", held, err)
	}

	checkCertificateSchema(t, s)

	// A re-issue of the migrated kind revokes the migrated certificate.
	old, err := s.IssueCertificate(ctx, Certificate{ID: "GP-EEEE-EEEE-EEEE", UserID: 1, Kind: "tier6", NameOnCert: "Alice",
		IssuedAt: t0, Payload: []byte(`{}`), Signature: []byte("s")})
	if err != nil || old != "GP-BBBB-BBBB-BBBB" {
		t.Errorf("re-issue of tier6 = %q, %v", old, err)
	}
	if fk := foreignKeyCheck(t, s); fk != "" {
		t.Errorf("foreign_key_check after a re-issue: %s", fk)
	}
}

// checkCertificateSchema checks the rebuilt table: the CHECK takes tier1 to
// tier10 and course, both indexes exist and work, the foreign keys point
// at the right tables and hold, and the old table is gone.
func checkCertificateSchema(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	insert := func(id, kind string) error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, payload, signature)
			VALUES (?, 1, ?, 'x', '2026-10-01T00:00:00.000Z', '{}', x'00')`, id, kind)
		return err
	}
	for i, kind := range []string{"tier1", "tier5", "tier8", "tier9", "tier10"} {
		id := "GP-CHK" + string(rune('A'+i))
		if err := insert(id, kind); err != nil {
			t.Errorf("kind %s refused: %v", kind, err)
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM certificates WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"tier0", "tier11", "tier01", "Tier1", "other", ""} {
		if err := insert("GP-BAD", kind); err == nil || !strings.Contains(err.Error(), "CHECK") {
			t.Errorf("kind %q: %v, want a CHECK failure", kind, err)
		}
	}

	indexes := map[string]string{}
	rows, err := queryAll(ctx, s.db, func(r scanner) ([2]string, error) {
		var v [2]string
		return v, r.Scan(&v[0], &v[1])
	}, `SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = 'certificates' AND sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		indexes[r[0]] = r[1]
	}
	if got := slices.Sorted(maps.Keys(indexes)); !slices.Equal(got, []string{"certificates_user", "certificates_valid"}) {
		t.Errorf("indexes = %v", got)
	}
	if !strings.Contains(indexes["certificates_valid"], "UNIQUE") || !strings.Contains(indexes["certificates_valid"], "WHERE revoked_at IS NULL") {
		t.Errorf("certificates_valid = %s", indexes["certificates_valid"])
	}
	// The partial unique index refuses a second valid certificate of a kind.
	if err := insert("GP-TWIN", "course"); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("a second valid course certificate: %v", err)
	}

	type fk struct{ table, from, to string }
	fks, err := queryAll(ctx, s.db, func(r scanner) (fk, error) {
		var f fk
		return f, r.Scan(&f.table, &f.from, &f.to)
	}, `SELECT "table", "from", "to" FROM pragma_foreign_key_list('certificates') ORDER BY "from"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []fk{{"certificates", "replaced_by", "id"}, {"users", "user_id", "id"}}; !slices.Equal(fks, want) {
		t.Errorf("foreign keys = %v, want %v", fks, want)
	}
	if got := foreignKeyCheck(t, s); got != "" {
		t.Errorf("foreign_key_check: %s", got)
	}
	var leftovers int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'certificates_v1%' OR sql LIKE '%certificates_v1%'`).Scan(&leftovers); err != nil || leftovers != 0 {
		t.Errorf("%d schema objects name certificates_v1, %v", leftovers, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE certificates SET replaced_by = 'GP-NOPE' WHERE id = 'GP-CCCC-CCCC-CCCC'`); err == nil {
		t.Error("replaced_by accepted an unknown certificate")
	}
}

// foreignKeyCheck returns the violations PRAGMA foreign_key_check finds.
func foreignKeyCheck(t *testing.T, s *Store) string {
	t.Helper()
	rows, err := queryAll(context.Background(), s.db, func(r scanner) (string, error) {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		err := r.Scan(&table, &rowid, &parent, &fkid)
		return table + "->" + parent, err
	}, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(rows, ", ")
}

// TestMigrationCourseV2Fresh: on a new database the migration changes no
// data, and the table ends up as in a migrated one.
func TestMigrationCourseV2Fresh(t *testing.T) {
	ctx := context.Background()
	fresh, _ := newStore(t)
	if v, err := fresh.SchemaVersion(ctx); err != nil || v != 3 {
		t.Fatalf("SchemaVersion = %d, %v", v, err)
	}
	var n int
	if err := fresh.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM certificates) + (SELECT COUNT(*) FROM code) +
		(SELECT COUNT(*) FROM progress) + (SELECT COUNT(*) FROM reveals) + (SELECT COUNT(*) FROM users)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("a fresh database has %d rows, %v", n, err)
	}

	empty, err := Open(ctx, openAtVersion(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = empty.Close() })
	schema := func(s *Store) []string {
		out, err := queryAll(ctx, s.db, func(r scanner) (string, error) {
			var q string
			return q, r.Scan(&q)
		}, `SELECT sql FROM sqlite_master WHERE tbl_name = 'certificates' AND sql IS NOT NULL ORDER BY name`)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if a, b := schema(fresh), schema(empty); !slices.Equal(a, b) || len(a) != 3 {
		t.Errorf("fresh schema\n%v\nmigrated schema\n%v", a, b)
	}
	// The users table does not exist without a row in it; reuse the
	// schema checks with a user.
	if _, err := fresh.db.ExecContext(ctx, `INSERT INTO users (id, username, display_name, email, webauthn_handle, created_at)
		VALUES (1, 'alice', 'Alice', 'alice@example.com', x'01', '2026-09-01T08:00:00.000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.db.ExecContext(ctx, `INSERT INTO certificates (id, user_id, kind, name_on_cert, issued_at, payload, signature)
		VALUES ('GP-CCCC-CCCC-CCCC', 1, 'course', 'Alice', '2026-10-01T00:00:00.000Z', '{}', x'00')`); err != nil {
		t.Fatal(err)
	}
	checkCertificateSchema(t, fresh)
}

// TestMigrationCourseV2MatchesRenames keeps the SQL and the course's rename
// table in step: every table renames exactly the 25 pairs.
func TestMigrationCourseV2MatchesRenames(t *testing.T) {
	body, err := migrations.ReadFile("migrations/0003_course_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	want := course.Renames()
	when := regexp.MustCompile(`WHEN '([0-9]{2}-[a-z]+)' THEN '([0-9]{2}-[a-z]+)'`)
	blocks := strings.Split(string(body), "UPDATE ")
	tables := 0
	for _, b := range blocks {
		table, _, _ := strings.Cut(b, " ")
		if table != "code" && table != "progress" && table != "reveals" {
			continue
		}
		tables++
		got := map[string]string{}
		for _, m := range when.FindAllStringSubmatch(b, -1) {
			got[m[1]] = m[2]
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: the CASE renames %v, want %v", table, got, want)
		}
		_, in, _ := strings.Cut(b, "WHERE exercise IN (")
		in, _, _ = strings.Cut(in, ")")
		var list []string
		for _, q := range strings.Split(in, ",") {
			list = append(list, strings.Trim(strings.TrimSpace(q), "'"))
		}
		if !slices.Equal(slices.Sorted(slices.Values(list)), slices.Sorted(maps.Keys(want))) {
			t.Errorf("%s: WHERE lists %v", table, list)
		}
	}
	if tables != 3 {
		t.Errorf("found %d table updates, want 3", tables)
	}
}
