package cert

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenderSamples writes sample PDFs for a visual check when
// CERT_SAMPLES_DIR is set: go test -run TestRenderSamples ./internal/cert/
func TestRenderSamples(t *testing.T) {
	dir := os.Getenv("CERT_SAMPLES_DIR")
	if dir == "" {
		t.Skip("CERT_SAMPLES_DIR not set")
	}
	c := loadCourse(t)
	signer, err := NewSigner(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)
	samples := []struct {
		file     string
		kind     Kind
		name     string
		stats    bool
		replaced string
	}{
		{"tier2.pdf", TierKind(2), "Yornik Heyl", false, ""},
		{"tier2-stats.pdf", TierKind(2), "Yornik Heyl", true, ""},
		{"tier1.pdf", TierKind(1), "Ada Lovelace", false, ""},
		{"course.pdf", KindCourse, "Grace Brewster Murray Hopper", false, ""},
		{"course-stats.pdf", KindCourse, "Grace Hopper", true, ""},
		{"tier5-long-name.pdf", TierKind(5), "Maximiliane Johanna Wilhelmina von Hohenzollern-Sigmaringen", true, ""},
		{"tier3-revoked.pdf", TierKind(3), "Linus Torvalds", false, "GP-7Q2M-K9XD-3HFA"},
	}
	for _, s := range samples {
		exs, _ := Exercises(c, s.kind)
		var slugs []string
		activity := map[string]Activity{}
		for i, e := range exs {
			slugs = append(slugs, e.Slug)
			activity[e.Slug] = Activity{ActiveSeconds: int64(3600*(i%4) + 1200*i + 900), Runs: 4 + 3*i, Levels: []int{1, 2, 3, 4, 5, 6}[:i%5]}
		}
		var st *Stats
		if s.stats {
			st = BuildStats(slugs, activity)
		}
		crt, err := signer.Issue(Request{Kind: s.kind, Name: s.name, Exercises: slugs, Stats: st, IssuedAt: issued})
		if err != nil {
			t.Fatal(err)
		}
		d, err := NewDocument(c, crt.Payload, "https://go-practice.dev", signer.Fingerprint())
		if err != nil {
			t.Fatal(err)
		}
		d.Revoked, d.ReplacedBy = s.replaced != "", s.replaced
		var buf bytes.Buffer
		if err := Render(&buf, d); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.file), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
