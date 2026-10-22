package cert

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/course"
)

// Coverage is what a signed payload covers, read through the loaded
// course. The payload's exercise list is the truth: a certificate of the
// first course names old slugs and an old kind, stays valid, and renders
// as what its exercises are now. An old "tier1" covers 01-logfmt,
// 02-accesslog and 03-latency, which are tier 6 of the course of 50.
type Coverage struct {
	Kind Kind
	// Exercises are the payload's exercises that the course knows, under
	// their current slugs, in course order.
	Exercises []*course.Exercise
	// Unknown are the payload's slugs that the course knows under no name.
	Unknown []string
	// Tier is the tier a tier certificate covers: the tier whose exercises
	// are its exercises, or the 1 tier that holds them all. nil for a
	// course certificate, or when the exercises span tiers.
	Tier *course.Tier
	// Tiers are the tiers that hold the exercises, in order.
	Tiers []*course.Tier
	// Stats is the payload's activity per exercise, by current slug; nil
	// when the payload has none.
	Stats map[string]ExerciseStats
	// Renamed is true when the payload names exercises by slugs of the
	// first course. Its PDF, printed then, shows the old numbers.
	Renamed bool
	// Signed is the payload's own exercise numbers, from its slugs, in
	// order: 1, 2, 3 for an old tier1.
	Signed []int
}

// Interpret reads a payload through the course's renames.
func Interpret(c *course.Course, p Payload) Coverage {
	cv := Coverage{Kind: p.Kind}
	seen := map[string]bool{}
	for _, slug := range p.Exercises {
		if n, ok := slugNumber(slug); ok {
			cv.Signed = append(cv.Signed, n)
		}
		e, renamed, ok := c.Resolve(slug)
		cv.Renamed = cv.Renamed || renamed
		if !ok {
			cv.Unknown = append(cv.Unknown, slug)
			continue
		}
		if !seen[e.Slug] {
			seen[e.Slug] = true
			cv.Exercises = append(cv.Exercises, e)
		}
	}
	slices.SortFunc(cv.Exercises, func(a, b *course.Exercise) int { return a.Number - b.Number })
	slices.Sort(cv.Signed)
	for _, t := range c.Tiers {
		if slices.ContainsFunc(t.Exercises, func(e *course.Exercise) bool { return seen[e.Slug] }) {
			cv.Tiers = append(cv.Tiers, t)
		}
	}
	if p.Kind != KindCourse && len(cv.Tiers) == 1 {
		cv.Tier = cv.Tiers[0]
	}
	if p.Stats != nil {
		cv.Stats = make(map[string]ExerciseStats, len(p.Stats.Exercises))
		for _, s := range p.Stats.Exercises {
			if cur := c.CurrentSlug(s.Slug); cur != "" {
				cv.Stats[cur] = s
			}
		}
	}
	return cv
}

// Slugs returns the current slugs of the covered exercises.
func (cv Coverage) Slugs() []string {
	out := make([]string, len(cv.Exercises))
	for i, e := range cv.Exercises {
		out[i] = e.Slug
	}
	return out
}

// Title is the certificate's name for people: "Tier 6, Forensics"
// or "The complete course". A tier certificate whose exercises span tiers
// falls back to the number in its kind.
func (cv Coverage) Title() string {
	switch {
	case cv.Kind == KindCourse:
		return courseTitle
	case cv.Tier != nil:
		return cv.Tier.Label()
	}
	if n, ok := cv.Kind.Tier(); ok {
		return fmt.Sprintf("Tier %d", n)
	}
	return string(cv.Kind)
}

// Summary counts what the certificate covers: "50 exercises in 10 tiers",
// "3 exercises".
func (cv Coverage) Summary() string {
	if cv.Kind != KindCourse {
		return plural(len(cv.Exercises), "exercise", "exercises")
	}
	return plural(len(cv.Exercises), "exercise", "exercises") + " in " + plural(len(cv.Tiers), "tier", "tiers")
}

// Range is the exercises as a range, "26 to 28"; "" when there are none.
func (cv Coverage) Range() string {
	if len(cv.Exercises) == 0 {
		return ""
	}
	return fmt.Sprintf("%02d to %02d", cv.Exercises[0].Number, cv.Exercises[len(cv.Exercises)-1].Number)
}

// IssuedAs says what a certificate of the first course was when it was
// issued, next to how the current course reads it: its PDF of then shows
// the old tier and numbers, and a reader who compares that paper with the
// verify page must see why they differ. "" for a payload of the current
// course.
func (cv Coverage) IssuedAs() string {
	if !cv.Renamed || len(cv.Signed) == 0 || len(cv.Exercises) == 0 {
		return ""
	}
	signed := numberRange(cv.Signed[0], cv.Signed[len(cv.Signed)-1])
	now := numberRange(cv.Exercises[0].Number, cv.Exercises[len(cv.Exercises)-1].Number)
	if cv.Kind == KindCourse {
		first, last := cv.Tiers[0].N, cv.Tiers[len(cv.Tiers)-1].N
		return fmt.Sprintf("Issued for the first course: %s in %s, numbered %s. The current course numbers them %s, in tiers %d to %d.",
			plural(len(cv.Signed), "exercise", "exercises"), plural(len(cv.Tiers), "tier", "tiers"), signed, now, first, last)
	}
	n, _ := cv.Kind.Tier()
	where := ""
	if cv.Tier != nil {
		where = ", in " + cv.Tier.Label()
	}
	return fmt.Sprintf("Issued as Tier %d of the first course, exercises %s. The current course numbers them %s%s.", n, signed, now, where)
}

// numberRange is "01 to 03", or "07" for 1 exercise.
func numberRange(first, last int) string {
	if first == last {
		return fmt.Sprintf("%02d", first)
	}
	return fmt.Sprintf("%02d to %02d", first, last)
}

// slugNumber is the number a slug starts with: 7 for "07-retry".
func slugNumber(slug string) (int, bool) {
	num, _, ok := strings.Cut(slug, "-")
	if !ok || len(num) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(num)
	return n, err == nil
}

// Current reports whether a certificate of kind k with this coverage
// covers exactly what k covers in the loaded course. A certificate that is
// not current is outdated: it stays valid, and the learner may claim k
// again, which replaces it.
func Current(c *course.Course, k Kind, cv Coverage) bool {
	want, err := Exercises(c, k)
	if err != nil || len(cv.Unknown) > 0 {
		return false
	}
	return slices.Equal(cv.Exercises, want)
}

// courseTitle is the name of the course certificate.
const courseTitle = "The complete course"
