// Package cert issues and verifies the course certificates: 1 per tier and
// 1 for the whole course. A certificate is a canonical JSON payload signed
// with Ed25519; the PDF and the public verify page show it. The signing
// key is generated at first start and kept in the database's settings.
package cert

import (
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yornik/go-practice-web/internal/course"
)

// Kind is what a certificate covers: "tier1" to "tier5", or "course".
type Kind string

// KindCourse is the certificate for the whole course.
const KindCourse Kind = "course"

// TierKind returns the kind for tier n.
func TierKind(n int) Kind { return Kind("tier" + strconv.Itoa(n)) }

// Tier returns the tier number of a tier kind.
func (k Kind) Tier() (int, bool) {
	s, ok := strings.CutPrefix(string(k), "tier")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || strconv.Itoa(n) != s {
		return 0, false
	}
	return n, true
}

// ErrKind is returned for a kind the course does not have.
var ErrKind = errors.New("cert: unknown certificate kind")

// Kinds returns every kind the course offers: the tiers in order, then the
// course.
func Kinds(c *course.Course) []Kind {
	out := make([]Kind, 0, len(c.Tiers)+1)
	for _, t := range c.Tiers {
		out = append(out, TierKind(t.N))
	}
	return append(out, KindCourse)
}

// ParseKind checks s against the course's kinds.
func ParseKind(c *course.Course, s string) (Kind, error) {
	k := Kind(s)
	if slices.Contains(Kinds(c), k) {
		return k, nil
	}
	return "", fmt.Errorf("%w: %q", ErrKind, s)
}

// Exercises returns the exercises a kind covers, in course order.
func Exercises(c *course.Course, k Kind) ([]*course.Exercise, error) {
	if k == KindCourse {
		return slices.Clone(c.Exercises), nil
	}
	if n, ok := k.Tier(); ok {
		if t, ok := c.Tier(n); ok {
			return slices.Clone(t.Exercises), nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrKind, k)
}

// Title is the name of a kind for people, for example
// "Tier 2: Streams, state, time, types, errors and locks".
func Title(c *course.Course, k Kind) (string, error) {
	if k == KindCourse {
		return "The complete course", nil
	}
	if n, ok := k.Tier(); ok {
		if t, ok := c.Tier(n); ok {
			return fmt.Sprintf("Tier %d: %s", t.N, t.Theme), nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrKind, k)
}

// Eligibility is a user's state for 1 kind of certificate.
type Eligibility struct {
	Kind      Kind
	Title     string
	Exercises []string // the slugs the kind covers
	Passed    int      // how many of them the user passed
	Eligible  bool     // all of them are passed
}

// Eligibilities returns the state of every kind for a user who passed the
// given exercise slugs. A tier needs every exercise of the tier, the course
// needs all exercises.
func Eligibilities(c *course.Course, passed []string) []Eligibility {
	done := make(map[string]bool, len(passed))
	for _, s := range passed {
		done[s] = true
	}
	var out []Eligibility
	for _, k := range Kinds(c) {
		exs, _ := Exercises(c, k) // k comes from Kinds
		title, _ := Title(c, k)
		e := Eligibility{Kind: k, Title: title}
		for _, ex := range exs {
			e.Exercises = append(e.Exercises, ex.Slug)
			if done[ex.Slug] {
				e.Passed++
			}
		}
		e.Eligible = len(exs) > 0 && e.Passed == len(exs)
		out = append(out, e)
	}
	return out
}

// Eligible reports whether a user who passed the given slugs may claim k.
func Eligible(c *course.Course, k Kind, passed []string) (bool, error) {
	if _, err := ParseKind(c, string(k)); err != nil {
		return false, err
	}
	for _, e := range Eligibilities(c, passed) {
		if e.Kind == k {
			return e.Eligible, nil
		}
	}
	return false, nil
}

// crockford is Crockford's base32 alphabet: no I, L, O or U, so an ID read
// aloud or typed from paper has no look-alikes.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// idSymbols is the number of random symbols in an ID: 60 bits.
const idSymbols = 12

// NewID returns a random certificate ID like "GP-7Q2M-K9XD-3HFA".
func NewID() string {
	var b [idSymbols]byte
	// crypto/rand.Read never fails on supported platforms.
	_, _ = rand.Read(b[:])
	var s strings.Builder
	s.WriteString("GP")
	for i, v := range b {
		if i%4 == 0 {
			s.WriteByte('-')
		}
		// 256 is a multiple of 32, so the low 5 bits are uniform.
		s.WriteByte(crockford[v&31])
	}
	return s.String()
}

// ErrID is returned for a string that is not a certificate ID.
var ErrID = errors.New("cert: not a certificate ID")

// ParseID normalises a typed or scanned ID: case, spaces and hyphens do not
// matter, and the look-alikes I, L and O read as 1, 1 and 0.
func ParseID(s string) (string, error) {
	var sym []byte
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch r {
		case '-', ' ':
			continue
		case 'I', 'L':
			r = '1'
		case 'O':
			r = '0'
		}
		if r > unicode.MaxASCII {
			return "", fmt.Errorf("%w: %q", ErrID, s)
		}
		sym = append(sym, byte(r))
	}
	if len(sym) != 2+idSymbols || string(sym[:2]) != "GP" {
		return "", fmt.Errorf("%w: %q", ErrID, s)
	}
	var out strings.Builder
	out.WriteString("GP")
	for i, c := range sym[2:] {
		if strings.IndexByte(crockford, c) < 0 {
			return "", fmt.Errorf("%w: %q", ErrID, s)
		}
		if i%4 == 0 {
			out.WriteByte('-')
		}
		out.WriteByte(c)
	}
	return out.String(), nil
}

// MaxNameRunes limits the printed name so that it fits on 1 line.
const MaxNameRunes = 70

// ErrName is returned for a name that cannot go on a certificate.
var ErrName = errors.New("cert: invalid name")

// NameError says why CleanName refused a name. It wraps ErrName.
type NameError struct {
	reason  string // for logs: "length", "not UTF-8", "not printable", "no glyph"
	Missing []rune // the characters the certificate fonts cannot draw
	r       rune   // the character that does not print
}

func (e *NameError) Error() string {
	switch {
	case len(e.Missing) > 0:
		return fmt.Sprintf("cert: invalid name: the certificate font cannot print %q", string(e.Missing))
	case e.reason == "not printable":
		return fmt.Sprintf("cert: invalid name: %U is not printable", e.r)
	case e.reason == "length":
		return fmt.Sprintf("cert: invalid name: use 1 to %d characters", MaxNameRunes)
	}
	return "cert: invalid name: " + e.reason
}

func (e *NameError) Unwrap() error { return ErrName }

// Message is the error for the learner: what is wrong and what to do.
func (e *NameError) Message() string {
	switch {
	case len(e.Missing) > 0:
		return fmt.Sprintf("The PDF cannot print %s. Use Latin, Greek or Cyrillic letters.", quoteRunes(e.Missing))
	case e.reason == "not printable":
		return "The name has a character that does not print. Use letters, spaces and punctuation."
	case e.reason == "length":
		return fmt.Sprintf("Use 1 to %d characters.", MaxNameRunes)
	}
	return "Use letters, spaces and punctuation."
}

// quoteRunes lists runes as "漢" or "漢 and 字".
func quoteRunes(rs []rune) string {
	seen := map[rune]bool{}
	var parts []string
	for _, r := range rs {
		if !seen[r] {
			seen[r] = true
			parts = append(parts, string(r))
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// CleanName trims and collapses spaces, and checks that the name is
// printable text of 1 to MaxNameRunes characters that the certificate font
// has glyphs for. Its errors are *NameError.
func CleanName(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", &NameError{reason: "not UTF-8"}
	}
	name := strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(name)
	if n == 0 || n > MaxNameRunes {
		return "", &NameError{reason: "length"}
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return "", &NameError{reason: "not printable", r: r}
		}
	}
	if missing := missingGlyphs(name); len(missing) > 0 {
		return "", &NameError{reason: "no glyph", Missing: missing}
	}
	return name, nil
}
