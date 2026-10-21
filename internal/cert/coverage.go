package cert

import (
	"fmt"
	"slices"

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
}

// Interpret reads a payload through the course's renames.
func Interpret(c *course.Course, p Payload) Coverage {
	cv := Coverage{Kind: p.Kind}
	seen := map[string]bool{}
	for _, slug := range p.Exercises {
		e, _, ok := c.Resolve(slug)
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

// Title is the certificate's name for people: "Tier 6 · The morning after"
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
