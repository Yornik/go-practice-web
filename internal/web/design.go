package web

import (
	"errors"
	"fmt"
	"html/template"
	"strings"

	"github.com/yornik/go-practice-web/internal/course"
)

// The template helpers of the Vesper Labs design (.design/VESPER.md): the
// slashed zero, the partials' arguments and the skyline. The staff badge
// is part of the story (story.go).

// zero sets a value for Saira, whose 0 is the same oval as its O. The
// whole run goes in gpw-zs, so a flex or grid cell never opens a gap
// inside "07", and each 0 in gpw-z, whose ::after draws the slash
// (VESPER.md 3.3). Every Saira string with a digit goes through it. The
// value is text: it is escaped here.
func zero(v any) template.HTML {
	s := fmt.Sprint(v)
	var b strings.Builder
	b.WriteString(`<span class="gpw-zs">`)
	for {
		i := strings.IndexByte(s, '0')
		if i < 0 {
			break
		}
		b.WriteString(template.HTMLEscapeString(s[:i]))
		b.WriteString(`<span class="gpw-z">0</span>`)
		s = s[i+1:]
	}
	b.WriteString(template.HTMLEscapeString(s))
	b.WriteString(`</span>`)
	return template.HTML(b.String()) //nolint:gosec // every piece of text is escaped above
}

// dict builds the argument of a partial that takes more than 1 value:
// {{template "tag" (dict "N" 2 "Name" "The Key" "State" "on")}}.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict: an odd number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
		}
		m[k] = pairs[i+1]
	}
	return m, nil
}

// gatePages are the pages of the email gate. They show the public header
// with "Sign out" (VESPER.md 6.5) also to a verified user, who can open
// verify-email's link again. Every other page does so only for a user
// whose address is not verified yet (render).
var gatePages = map[string]bool{"check-email": true, "verify-email": true}

// The skyline (VESPER.md 6.15): 1 tower per exercise, as tall as its
// estimate, 6 h the full height. The heights, the plate offsets and the
// group widths are classes on a fixed scale in site.css (gpw-h5 to
// gpw-h60 in tenths of an hour, gpw-g-top5 to gpw-g-top60, gpw-g1 to
// gpw-g10), so the strict CSP needs no style attribute and no file is
// generated at start; TestSkylineScale keeps the scale whole. The scale
// starts at 5 because gpw-h1 and gpw-h2 are headlines, and a group stops
// at 10 because gpw-g12 is the 12-column grid.
const (
	skyMinTenths  = 5  // half an hour
	skyFullTenths = 60 // 6 h
	skyMaxGroup   = 10
)

// skyline is the course as a city: 1 group per tier, 1 tower per
// exercise. The "skyline" template function builds one for a partial call;
// the story builds the learner's own (story.go).
type skyline struct {
	Class   string // the kind of skyline as a class (skyKinds)
	Label   string // the accessible name of the 1 role="img"
	Mini    bool   // no plates and no year labels
	Caption string // the header's labels: "6 in the Ledger, 07 on call"
	CurWord string // the legend's word for the amber tower: "On call" or "Next"
	// OffWord is the legend's word for an outlined tower: "Not started",
	// "Not in the Ledger yet" when some of them started, or "" when no
	// tower is an outline.
	OffWord string
	Years   []skyYear
}

type skyYear struct {
	N     int
	Tiers []skyGroup
}

// skyGroup is 1 tier: its plate and its towers.
type skyGroup struct {
	*course.Tier
	Plate  string // "", "on" (every tower in the Ledger) or "cur" (it holds the amber tower)
	Top    int    // its tallest tower in tenths of an hour
	Towers []skyTower
}

// Size is the group's width class: its number of towers, at most
// skyMaxGroup.
func (g skyGroup) Size() int { return min(len(g.Towers), skyMaxGroup) }

// Class is the group's classes: its width and its plate's height, as
// classes of the fixed scale (no style attributes under the CSP).
func (g skyGroup) Class() string {
	return fmt.Sprintf("gpw-sky-g gpw-g%d gpw-g-top%d", g.Size(), g.Top)
}

// PlateClass is the plate's classes.
func (g skyGroup) PlateClass() string {
	if g.Plate == "" {
		return "gpw-sky-plate"
	}
	return "gpw-sky-plate gpw-sky-plate-" + g.Plate
}

// skyTower is 1 exercise.
type skyTower struct {
	*course.Exercise
	State string // "", "on" (in the Ledger) or "cur" (on call, or next)
	H     int    // the estimate in tenths of an hour, on the scale
	Roof  bool   // the tallest tower of its tier
}

// Class is the tower's classes: "gpw-tw gpw-h15 gpw-tw-on gpw-tw-roof".
func (t skyTower) Class() string {
	c := fmt.Sprintf("gpw-tw gpw-h%d", t.H)
	if t.State != "" {
		c += " gpw-tw-" + t.State
	}
	if t.Roof {
		c += " gpw-tw-roof"
	}
	return c
}

// skyKinds maps a kind to its class: the landing with the reflection, the
// dashboard, the Key's rail on the certificates page, and the admin's mini.
var skyKinds = map[string]string{"landing": "gpw-sky-refl", "dash": "gpw-sky-dash", "key": "gpw-sky-key", "mini": "gpw-sky-mini"}

// skylineFunc is the "skyline" template function: the whole course as
// kind. states maps a slug to "passed" or "on" (a solid tower) and "here"
// or "cur" (the amber tower); any other state is an outline.
func skylineFunc(c *course.Course, states map[string]string, kind, label string) (skyline, error) {
	class, ok := skyKinds[kind]
	if !ok {
		return skyline{}, fmt.Errorf("skyline: unknown kind %q", kind)
	}
	sky := drawSkyline(c, func(ex *course.Exercise) string {
		switch states[ex.Slug] {
		case "passed", "on":
			return "on"
		case "here", "cur":
			return "cur"
		}
		return ""
	})
	sky.Class, sky.Label, sky.Mini = class, label, kind == "mini"
	return sky, nil
}

// buildSkyline draws the course with the towers in on solid and cur amber,
// as the dashboard's kind.
func buildSkyline(c *course.Course, on map[string]bool, cur string) skyline {
	sky := drawSkyline(c, func(ex *course.Exercise) string {
		switch {
		case ex.Slug == cur:
			return "cur"
		case on[ex.Slug]:
			return "on"
		}
		return ""
	})
	sky.Class = skyKinds["dash"]
	return sky
}

// drawSkyline lays out the towers; state gives each tower "", "on" or
// "cur". A plate is "cur" when its tier holds an amber tower, else "on"
// when every tower is solid.
func drawSkyline(c *course.Course, state func(*course.Exercise) string) skyline {
	var sky skyline
	for _, t := range c.Tiers {
		g := skyGroup{Tier: t}
		tallest := -1
		for i, ex := range t.Exercises {
			tw := skyTower{Exercise: ex, State: state(ex), H: skyTenths(ex.Hours)}
			if tw.State == "cur" {
				g.Plate = "cur"
			}
			if tallest < 0 || tw.H > g.Towers[tallest].H {
				tallest = i
			}
			g.Top = max(g.Top, tw.H)
			g.Towers = append(g.Towers, tw)
		}
		if tallest < 0 {
			continue
		}
		g.Towers[tallest].Roof = true
		if g.Plate == "" {
			all := true
			for _, tw := range g.Towers {
				all = all && tw.State == "on"
			}
			if all {
				g.Plate = "on"
			}
		}
		if n := len(sky.Years); n == 0 || sky.Years[n-1].N != t.Year() {
			sky.Years = append(sky.Years, skyYear{N: t.Year()})
		}
		y := &sky.Years[len(sky.Years)-1]
		y.Tiers = append(y.Tiers, g)
	}
	return sky
}

// skyTenths is a tower's height class: the estimate in tenths of an hour,
// on the scale of site.css.
func skyTenths(h float64) int {
	return max(skyMinTenths, min(skyFullTenths, int(h*10+0.5)))
}
