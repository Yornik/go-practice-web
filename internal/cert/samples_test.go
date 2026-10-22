package cert

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
)

// TestRenderSamples writes the 4 proofs of .design/vesper-v2/pdf.html
// with the same data, for a look by eye next to the frame's proofs, when
// CERT_SAMPLES_DIR is set:
//
//	CERT_SAMPLES_DIR=/tmp/proofs GO_PRACTICE_COURSE=$PWD/course go test -run TestRenderSamples ./internal/cert/
//
// GO_PRACTICE_COURSE gives the real titles; without it the fixture's.
func TestRenderSamples(t *testing.T) {
	dir := os.Getenv("CERT_SAMPLES_DIR")
	if dir == "" {
		t.Skip("CERT_SAMPLES_DIR not set")
	}
	c := loadCourse(t)
	if p := os.Getenv("GO_PRACTICE_COURSE"); p != "" {
		var err error
		if c, err = course.Load(p); err != nil {
			t.Fatal(err)
		}
	}
	const fingerprint = "3F2A 91C4 0B7E D215"
	slugs := func(k Kind) []string {
		exs, err := Exercises(c, k)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range exs {
			out = append(out, e.Slug)
		}
		return out
	}
	samples := []struct {
		file     string
		p        Payload
		replaced string
	}{
		{"1-tier.pdf", Payload{ID: "GP-6RVA-3KXN-Q8TD", Kind: "tier1", Name: "Łukasz Dvořák-Wiśniewska", IssuedAt: "2026-09-18T10:00:00Z", Exercises: slugs("tier1")}, ""},
		{"2-tier-revoked.pdf", Payload{ID: "GP-5K2P-W8DM-3QXR", Kind: "tier6", Name: "Łukasz Dvořák-Wiśniewska", IssuedAt: "2026-03-14T10:00:00Z", Exercises: slugs("tier6")}, "GP-8N4V-T2KC-9HWB"},
		{"3-course.pdf", Payload{ID: "GP-3TQM-7VXC-K4NB", Kind: KindCourse, Name: "Анна", IssuedAt: "2026-10-19T10:00:00Z", Exercises: slugs(KindCourse),
			Stats: &Stats{ActiveSeconds: 112*3600 + 40*60, Runs: 1264, Hints: 31, Explanations: 50, Walkthroughs: 22, Solutions: 3}}, ""},
		{"4-course-revoked.pdf", Payload{ID: "GP-2CJM-6YPN-3RTV", Kind: KindCourse, Name: longLatin, IssuedAt: "2026-10-02T10:00:00Z", Exercises: slugs(KindCourse)}, "GP-8LKD-5WQZ-4MHF"},
	}
	for _, s := range samples {
		d, err := NewDocument(c, s.p, "https://vesper-labs.org", fingerprint)
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
