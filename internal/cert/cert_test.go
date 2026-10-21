package cert

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yornik/go-practice-web/internal/store"
)

var idPattern = regexp.MustCompile(`^GP-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`)

func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	counts := map[byte]int{}
	for range 20000 {
		id := NewID()
		if !idPattern.MatchString(id) {
			t.Fatalf("NewID() = %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
		for _, c := range []byte(strings.ReplaceAll(id[3:], "-", "")) {
			counts[c]++
		}
		if got, err := ParseID(id); err != nil || got != id {
			t.Fatalf("ParseID(%q) = %q, %v", id, got, err)
		}
	}
	// Every symbol of the alphabet shows up: no bias towards a subset.
	if len(counts) != 32 {
		t.Errorf("IDs use %d symbols, want 32", len(counts))
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"GP-7Q2M-K9XD-3HFA", "GP-7Q2M-K9XD-3HFA"},
		{" gp-7q2m-k9xd-3hfa ", "GP-7Q2M-K9XD-3HFA"},
		{"GP7Q2MK9XD3HFA", "GP-7Q2M-K9XD-3HFA"},
		{"GP 7Q2M K9XD 3HFA", "GP-7Q2M-K9XD-3HFA"},
		{"GP-OOII-LLoo-1234", "GP-0011-1100-1234"},
		{"GP-7Q2M-K9XD-3HF", ""},
		{"GP-7Q2M-K9XD-3HFAA", ""},
		{"XX-7Q2M-K9XD-3HFA", ""},
		{"GP-7Q2M-K9XD-3HFU", ""}, // U is not in the alphabet
		{"GP-7Q2M-K9XD-3HF!", ""},
		{"GP-7Q2M-K9XD-3HFÄ", ""},
		{"", ""},
		{"../../etc/passwd", ""},
	}
	for _, tt := range tests {
		got, err := ParseID(tt.in)
		if tt.want == "" {
			if !errors.Is(err, ErrID) {
				t.Errorf("ParseID(%q) = %q, %v; want ErrID", tt.in, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseID(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestKinds(t *testing.T) {
	c := loadCourse(t)
	want := []Kind{"tier1", "tier2", "tier3", "tier4", "tier5", "course"}
	if got := Kinds(c); !slices.Equal(got, want) {
		t.Errorf("Kinds = %v", got)
	}
	tests := []struct {
		kind  string
		ok    bool
		tier  int
		count int
		title string
	}{
		{"tier1", true, 1, 3, "Tier 1"},
		{"tier2", true, 2, 6, "Tier 2"},
		{"tier5", true, 5, 6, "Tier 5"},
		{"course", true, 0, 25, "The complete course"},
		{"tier6", false, 0, 0, ""},
		{"tier0", false, 0, 0, ""},
		{"tier01", false, 0, 0, ""},
		{"Tier1", false, 0, 0, ""},
		{"", false, 0, 0, ""},
	}
	for _, tt := range tests {
		k, err := ParseKind(c, tt.kind)
		if (err == nil) != tt.ok {
			t.Errorf("ParseKind(%q) = %q, %v", tt.kind, k, err)
			continue
		}
		if !tt.ok {
			if !errors.Is(err, ErrKind) {
				t.Errorf("ParseKind(%q): %v, want ErrKind", tt.kind, err)
			}
			continue
		}
		if n, isTier := k.Tier(); n != tt.tier || isTier != (tt.tier > 0) {
			t.Errorf("%s.Tier() = %d, %v", k, n, isTier)
		}
		exs, err := Exercises(c, k)
		if err != nil || len(exs) != tt.count {
			t.Errorf("Exercises(%s) = %d, %v", k, len(exs), err)
		}
		if title, err := Title(c, k); err != nil || title != tt.title {
			t.Errorf("Title(%s) = %q, %v", k, title, err)
		}
	}
	if _, err := Exercises(c, "tier9"); !errors.Is(err, ErrKind) {
		t.Errorf("Exercises(tier9): %v", err)
	}
	if _, err := Title(c, "nope"); !errors.Is(err, ErrKind) {
		t.Errorf("Title(nope): %v", err)
	}
}

func TestEligibility(t *testing.T) {
	c := loadCourse(t)
	slugs := func(from, to int) []string {
		var out []string
		for _, e := range c.Exercises {
			if e.Number >= from && e.Number <= to {
				out = append(out, e.Slug)
			}
		}
		return out
	}
	tests := []struct {
		name     string
		passed   []string
		eligible []Kind
	}{
		{"nothing", nil, nil},
		{"2 of tier 1", slugs(1, 2), nil},
		{"tier 1", slugs(1, 3), []Kind{"tier1"}},
		{"tier 1 and most of tier 2", slugs(1, 8), []Kind{"tier1"}},
		{"tiers 1 and 2", slugs(1, 9), []Kind{"tier1", "tier2"}},
		{"tier 3 alone", slugs(10, 14), []Kind{"tier3"}},
		{"all but 25", slugs(1, 24), []Kind{"tier1", "tier2", "tier3", "tier4"}},
		{"everything", slugs(1, 25), []Kind{"tier1", "tier2", "tier3", "tier4", "tier5", "course"}},
		{"everything plus unknown slugs", append(slugs(1, 25), "99-extra", "../x"), []Kind{"tier1", "tier2", "tier3", "tier4", "tier5", "course"}},
	}
	for _, tt := range tests {
		var got []Kind
		for _, e := range Eligibilities(c, tt.passed) {
			if e.Eligible {
				got = append(got, e.Kind)
			}
			if e.Passed > len(e.Exercises) {
				t.Errorf("%s: %s passed %d of %d", tt.name, e.Kind, e.Passed, len(e.Exercises))
			}
		}
		if !slices.Equal(got, tt.eligible) {
			t.Errorf("%s: eligible %v, want %v", tt.name, got, tt.eligible)
		}
		for _, k := range Kinds(c) {
			ok, err := Eligible(c, k, tt.passed)
			if err != nil || ok != slices.Contains(tt.eligible, k) {
				t.Errorf("%s: Eligible(%s) = %v, %v", tt.name, k, ok, err)
			}
		}
	}
	if _, err := Eligible(c, "tier7", nil); !errors.Is(err, ErrKind) {
		t.Errorf("Eligible(tier7): %v", err)
	}
}

func TestBuildStats(t *testing.T) {
	got := BuildStats([]string{"01-a", "02-b", "03-c"}, map[string]Activity{
		"01-a":  {ActiveSeconds: 3600, Runs: 10, Levels: []int{1, 2}},
		"02-b":  {ActiveSeconds: 1800, Runs: 3, Levels: []int{1, 2, 3, 4, 5, 6}},
		"99-zz": {ActiveSeconds: 999, Runs: 99}, // not covered: ignored
	})
	want := &Stats{
		ActiveSeconds: 5400, Runs: 13, Hints: 5, Explanations: 1, Walkthroughs: 1, Solutions: 1,
		Exercises: []ExerciseStats{
			{Slug: "01-a", ActiveSeconds: 3600, Runs: 10, Hints: 2},
			{Slug: "02-b", ActiveSeconds: 1800, Runs: 3, Hints: 3, Explanation: true, Walkthrough: true, Solution: true},
			{Slug: "03-c"},
		},
	}
	if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
		t.Errorf("BuildStats =\n%+v\nwant\n%+v", got, want)
	}
}

func testSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(bytes.Repeat([]byte{42}, ed25519.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var issuedAt = time.Date(2026, 10, 19, 9, 30, 15, 123456789, time.FixedZone("CEST", 2*3600))

func TestIssueAndVerify(t *testing.T) {
	s := testSigner(t)
	stats := BuildStats([]string{"01-logfmt"}, map[string]Activity{"01-logfmt": {ActiveSeconds: 60, Runs: 2, Levels: []int{1}}})
	crt, err := s.Issue(Request{Kind: "tier1", Name: "  Ada   Lovelace <&> ", Exercises: []string{"01-logfmt", "02-accesslog", "03-latency"}, Stats: stats, IssuedAt: issuedAt})
	if err != nil {
		t.Fatal(err)
	}
	p := crt.Payload
	if !idPattern.MatchString(p.ID) || p.Name != "Ada Lovelace <&>" || p.IssuedAt != "2026-10-19T07:30:15Z" {
		t.Errorf("payload = %+v", p)
	}
	wantJSON := `{"id":"` + p.ID + `","kind":"tier1","name":"Ada Lovelace <&>","issuedAt":"2026-10-19T07:30:15Z",` +
		`"exercises":["01-logfmt","02-accesslog","03-latency"],"stats":{"activeSeconds":60,"runs":2,"hints":1,` +
		`"explanations":0,"walkthroughs":0,"solutions":0,"exercises":[{"slug":"01-logfmt","activeSeconds":60,` +
		`"runs":2,"hints":1,"explanation":false,"walkthrough":false,"solution":false}]}}`
	if string(crt.Canonical) != wantJSON {
		t.Errorf("canonical payload\n got %s\nwant %s", crt.Canonical, wantJSON)
	}
	again, _ := p.Canonical()
	if !bytes.Equal(again, crt.Canonical) {
		t.Error("Canonical is not deterministic")
	}

	got, err := s.Verify(crt.Canonical, crt.Signature)
	if err != nil || got.ID != p.ID || got.Name != p.Name || got.Stats == nil || got.Stats.Runs != 2 {
		t.Fatalf("Verify = %+v, %v", got, err)
	}
	if at, err := got.Issued(); err != nil || !at.Equal(issuedAt.Truncate(time.Second)) {
		t.Errorf("Issued = %v, %v", at, err)
	}
	// A verifier with only the published public key agrees.
	pub, _ := base64.StdEncoding.DecodeString(s.PublicKeyBase64())
	if _, err := Verify(pub, crt.Canonical, crt.Signature); err != nil {
		t.Errorf("Verify with the published key: %v", err)
	}

	// Without stats the field is absent, not null.
	plain, err := s.Issue(Request{Kind: KindCourse, Name: "Grace", Exercises: []string{"01-logfmt"}})
	if err != nil || bytes.Contains(plain.Canonical, []byte("stats")) {
		t.Errorf("payload without stats: %s, %v", plain.Canonical, err)
	}
	if plain.Payload.ID == p.ID {
		t.Error("2 certificates share an ID")
	}
	if _, err := s.Issue(Request{Kind: "tier1", Name: " ", Exercises: []string{"x"}}); !errors.Is(err, ErrName) {
		t.Errorf("Issue with a blank name: %v", err)
	}
	if _, err := s.Issue(Request{Kind: "tier1", Name: "Ada"}); err == nil {
		t.Error("Issue without exercises succeeded")
	}
}

func TestTamperDetection(t *testing.T) {
	s := testSigner(t)
	crt, err := s.Issue(Request{Kind: "tier2", Name: "Ada Lovelace", Exercises: []string{"04-grep", "05-slo"}, IssuedAt: issuedAt})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewSigner(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	flip := func(b []byte, i int) []byte {
		out := bytes.Clone(b)
		out[i] ^= 1
		return out
	}
	tests := []struct {
		name    string
		verify  func() (Payload, error)
		wantErr error
	}{
		{"other name", func() (Payload, error) {
			return s.Verify(bytes.Replace(crt.Canonical, []byte("Ada Lovelace"), []byte("Eve Mallory!"), 1), crt.Signature)
		}, ErrSignature},
		{"other kind", func() (Payload, error) {
			return s.Verify(bytes.Replace(crt.Canonical, []byte(`"tier2"`), []byte(`"tier5"`), 1), crt.Signature)
		}, ErrSignature},
		{"extra space", func() (Payload, error) {
			return s.Verify(append(bytes.Clone(crt.Canonical), ' '), crt.Signature)
		}, ErrSignature},
		{"flipped signature bit", func() (Payload, error) { return s.Verify(crt.Canonical, flip(crt.Signature, 10)) }, ErrSignature},
		{"short signature", func() (Payload, error) { return s.Verify(crt.Canonical, crt.Signature[:32]) }, ErrSignature},
		{"other key", func() (Payload, error) { return other.Verify(crt.Canonical, crt.Signature) }, ErrSignature},
		{"no key", func() (Payload, error) { return Verify(nil, crt.Canonical, crt.Signature) }, ErrSignature},
	}
	for _, tt := range tests {
		if _, err := tt.verify(); !errors.Is(err, tt.wantErr) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
		}
	}

	// A validly signed but non-canonical payload is refused too: the
	// verify page shows only what canonical bytes say.
	odd := []byte(`{"kind":"tier2","id":"GP-0000-0000-0000","name":"x","issuedAt":"2026-10-19T07:30:15Z","exercises":[]}`)
	if _, err := s.Verify(odd, ed25519.Sign(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{42}, 32)), odd)); err == nil {
		t.Error("Verify accepted a non-canonical payload")
	}
	extra := []byte(`{"id":"GP-0000-0000-0000","kind":"tier2","name":"x","issuedAt":"2026-10-19T07:30:15Z","exercises":[],"admin":true}`)
	if _, err := ParsePayload(extra); err == nil {
		t.Error("ParsePayload accepted an unknown field")
	}
}

// mapKeys is a KeyStore in memory.
type mapKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *mapKeys) SetSettingIfAbsent(_ context.Context, key, value string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[key]; ok {
		return v, nil
	}
	k.m[key] = value
	return value, nil
}

type failingKeys struct{}

func (failingKeys) SetSettingIfAbsent(context.Context, string, string) (string, error) {
	return "", errors.New("disk on fire")
}

func TestLoadSigner(t *testing.T) {
	ctx := context.Background()
	keys := &mapKeys{m: map[string]string{}}
	a, err := LoadSigner(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadSigner(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	if !a.PublicKey().Equal(b.PublicKey()) || a.Fingerprint() != b.Fingerprint() {
		t.Error("the second load generated a new key")
	}
	if len(keys.m) != 1 {
		t.Errorf("settings hold %d keys", len(keys.m))
	}
	if _, err := LoadSigner(ctx, failingKeys{}); err == nil {
		t.Error("LoadSigner ignored a store error")
	}
	if _, err := LoadSigner(ctx, &mapKeys{m: map[string]string{keySetting: "not base64!"}}); err == nil {
		t.Error("LoadSigner accepted a corrupt key")
	}
	if _, err := LoadSigner(ctx, &mapKeys{m: map[string]string{keySetting: base64.StdEncoding.EncodeToString([]byte("short"))}}); err == nil {
		t.Error("LoadSigner accepted a short key")
	}

	// With the real store, 2 concurrent first starts agree on 1 key.
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	var wg sync.WaitGroup
	fps := make([]string, 4)
	for i := range fps {
		wg.Go(func() {
			s, err := LoadSigner(ctx, st)
			if err != nil {
				t.Error(err)
				return
			}
			fps[i] = s.Fingerprint()
		})
	}
	wg.Wait()
	for _, fp := range fps {
		if fp != fps[0] || fp == "" {
			t.Fatalf("fingerprints differ: %q", fps)
		}
	}
}

func TestFingerprint(t *testing.T) {
	s := testSigner(t)
	fp := s.Fingerprint()
	if !regexp.MustCompile(`^[0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4}$`).MatchString(fp) {
		t.Errorf("Fingerprint = %q", fp)
	}
	other, _ := NewSigner(bytes.Repeat([]byte{43}, ed25519.SeedSize))
	if other.Fingerprint() == fp {
		t.Error("2 keys share a fingerprint")
	}
	if len(s.PublicKeyBase64()) != 44 {
		t.Errorf("PublicKeyBase64 = %q", s.PublicKeyBase64())
	}
	if _, err := NewSigner([]byte("short")); err == nil {
		t.Error("NewSigner accepted a short seed")
	}
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"Ada Lovelace", "Ada Lovelace", true},
		{"  Ada \t  Lovelace\n", "Ada Lovelace", true},
		{"Zoë Ångström-Çelik", "Zoë Ångström-Çelik", true},
		{"Łukasz Żółć", "Łukasz Żółć", true},
		{"Пётр Чайковский", "Пётр Чайковский", true},
		{"Łukasz Dvořák", "Łukasz Dvořák", true},
		{"Анна", "Анна", true},
		{"O'Brien & Sons", "O'Brien & Sons", true},
		{"", "", false},
		{"   ", "", false},
		{strings.Repeat("a", MaxNameRunes), strings.Repeat("a", MaxNameRunes), true},
		{strings.Repeat("a", MaxNameRunes+1), "", false},
		{"Ada\x00Lovelace", "", false},
		{"Ada\u202eecalevoL", "", false}, // right-to-left override
		{"李小龍", "", false},               // no CJK glyphs in the font
		{"Ada 🚀", "", false},
		{string([]byte{0xff, 0xfe}), "", false},
	}
	for _, tt := range tests {
		got, err := CleanName(tt.in)
		if tt.ok != (err == nil) || got != tt.want {
			t.Errorf("CleanName(%q) = %q, %v", tt.in, got, err)
		}
		if err != nil && !errors.Is(err, ErrName) {
			t.Errorf("CleanName(%q): %v, want ErrName", tt.in, err)
		}
	}
}

func TestNewDocument(t *testing.T) {
	c := loadCourse(t)
	s := testSigner(t)
	for _, k := range Kinds(c) {
		for _, withStats := range []bool{false, true} {
			exs, _ := Exercises(c, k)
			var slugs []string
			for _, e := range exs {
				slugs = append(slugs, e.Slug)
			}
			var st *Stats
			if withStats {
				st = BuildStats(slugs, map[string]Activity{slugs[0]: {ActiveSeconds: 7200, Runs: 5, Levels: []int{1}}})
			}
			crt, err := s.Issue(Request{Kind: k, Name: "Ada", Exercises: slugs, Stats: st, IssuedAt: issuedAt})
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewDocument(c, crt.Payload, "https://go-practice.dev/", s.Fingerprint())
			if err != nil {
				t.Fatal(err)
			}
			if d.VerifyURL != "https://go-practice.dev/verify/"+crt.Payload.ID || d.Count != len(slugs) || d.Fingerprint != s.Fingerprint() {
				t.Errorf("%s: document = %+v", k, d)
			}
			wantItems := len(slugs)
			if k == KindCourse {
				wantItems = len(c.Tiers)
			}
			if len(d.Items) != wantItems {
				t.Errorf("%s: %d items, want %d", k, len(d.Items), wantItems)
			}
			if withStats != (d.Items[0].Detail != "") {
				t.Errorf("%s stats=%v: first item detail %q", k, withStats, d.Items[0].Detail)
			}
			if withStats && !strings.HasPrefix(d.Items[0].Detail, "2 h") {
				t.Errorf("%s: detail %q", k, d.Items[0].Detail)
			}
		}
	}
	bad := Payload{ID: "GP-0000-0000-0000", Kind: "nope", Name: "x", IssuedAt: "2026-10-19T07:30:15Z"}
	if _, err := NewDocument(c, bad, "https://go-practice.dev", ""); !errors.Is(err, ErrKind) {
		t.Errorf("NewDocument(nope): %v", err)
	}
	bad.Kind, bad.IssuedAt = "tier1", "yesterday"
	if _, err := NewDocument(c, bad, "https://go-practice.dev", ""); err == nil {
		t.Error("NewDocument accepted a bad date")
	}
}

func TestRender(t *testing.T) {
	c := loadCourse(t)
	s := testSigner(t)
	tests := []struct {
		kind       Kind
		name       string
		stats      bool
		revoked    bool
		replacedBy string
	}{
		{"tier1", "Ada Lovelace", false, false, ""},
		{"tier3", "Ada Lovelace", true, false, ""},
		{KindCourse, "Grace Brewster Murray Hopper", true, false, ""},
		{"tier5", strings.Repeat("Wilhelmina ", 6), false, true, ""},
		{"tier2", "Łukasz Żółć", false, true, "GP-7Q2M-K9XD-3HFA"},
	}
	for _, tt := range tests {
		exs, _ := Exercises(c, tt.kind)
		var slugs []string
		for _, e := range exs {
			slugs = append(slugs, e.Slug)
		}
		var st *Stats
		if tt.stats {
			st = BuildStats(slugs, nil)
		}
		crt, err := s.Issue(Request{Kind: tt.kind, Name: tt.name, Exercises: slugs, Stats: st, IssuedAt: issuedAt})
		if err != nil {
			t.Fatal(err)
		}
		d, err := NewDocument(c, crt.Payload, "https://go-practice.dev", s.Fingerprint())
		if err != nil {
			t.Fatal(err)
		}
		d.Revoked, d.ReplacedBy = tt.revoked, tt.replacedBy
		var buf bytes.Buffer
		if err := Render(&buf, d); err != nil {
			t.Fatalf("%s: %v", tt.kind, err)
		}
		pdf := buf.Bytes()
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf[len(pdf)-32:], []byte("%%EOF")) {
			t.Errorf("%s: not a PDF: %q ... %q", tt.kind, pdf[:16], pdf[len(pdf)-16:])
		}
		// The verify URL is a link annotation, readable as plain text.
		if !bytes.Contains(pdf, []byte("/URI (https://go-practice.dev/verify/"+crt.Payload.ID+")")) {
			t.Errorf("%s: no link to the verify page", tt.kind)
		}
		// Subset fonts keep the file small enough to email.
		if len(pdf) > 400<<10 {
			t.Errorf("%s: PDF is %d KiB", tt.kind, len(pdf)>>10)
		}
	}
}

func TestQRMatrix(t *testing.T) {
	url := "https://go-practice.dev/verify/GP-7Q2M-K9XD-3HFA"
	m1, err := QRMatrix(url)
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := QRMatrix(url)
	if fmt.Sprint(m1) != fmt.Sprint(m2) {
		t.Error("QRMatrix is not deterministic")
	}
	other, _ := QRMatrix(strings.Replace(url, "3HFA", "3HFB", 1))
	if fmt.Sprint(m1) == fmt.Sprint(other) {
		t.Error("2 URLs give the same matrix")
	}
	n := len(m1)
	if n < 21 || (n-17)%4 != 0 {
		t.Fatalf("matrix size %d is not a QR version size", n)
	}
	// The 3 finder patterns: a dark 7×7 ring, a light ring, a dark 3×3 core.
	for _, corner := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
		for y := range 7 {
			for x := range 7 {
				ring := max(abs(x-3), abs(y-3))
				want := ring != 2
				if m1[corner[1]+y][corner[0]+x] != want {
					t.Fatalf("finder pattern at %v is wrong at (%d, %d)", corner, x, y)
				}
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestActivityText(t *testing.T) {
	tests := map[int64]string{0: "0 min", 59: "0 min", 360: "6 min", 3600: "1 h", 5400: "1 h 30 min", 78000: "21 h 40 min"}
	for sec, want := range tests {
		if got := duration(sec); got != want {
			t.Errorf("duration(%d) = %q, want %q", sec, got, want)
		}
	}
	if got := activityLine(ExerciseStats{ActiveSeconds: 3600, Runs: 1, Hints: 2}); got != "1 h, 1 test run, 2 hints" {
		t.Errorf("activityLine = %q", got)
	}
}

func TestNameFont(t *testing.T) {
	pdf, err := newPDF()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"Ada Lovelace":        fontManualBold,
		"Zoë Ångström-Çelik":  fontManualBold,
		"Łukasz Dvořák":       fontNameWide,
		"Анна":                fontNameWide,
		"Ada (Countess) King": fontNameWide,
	} {
		if got := nameFont(pdf, name); got != want {
			t.Errorf("nameFont(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestNameErrorMessage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "Use 1 to 70 characters."},
		{"Ada 漢字", "The PDF cannot print 漢 and 字. Use Latin, Greek or Cyrillic letters."},
		{"Ada\u0007", "The name has a character that does not print. Use letters, spaces and punctuation."},
	}
	for _, tt := range tests {
		_, err := CleanName(tt.in)
		var ne *NameError
		if !errors.As(err, &ne) || !errors.Is(err, ErrName) {
			t.Fatalf("CleanName(%q) = %v, want a *NameError", tt.in, err)
		}
		if got := ne.Message(); got != tt.want {
			t.Errorf("CleanName(%q) message = %q, want %q", tt.in, got, tt.want)
		}
	}
}
