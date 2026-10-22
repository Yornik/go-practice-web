package course

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// The site's story (the Vesper Labs story bible) gives each tier of the
// course of 50 a stamped tag on the learner's ring, a place in the story's
// calendar and a 1-line theme for its certificate. The course README holds
// only the names. The table is keyed by name, not number, so a tier that
// the story does not know (the first course had no names) gets no story
// instead of another tier's.
type tierStory struct {
	when  string // the months the tier spans: "December to 1 January"
	month string // the month in Meridian while the tier is open: "December"
	line  string // what the certificate says after the colon
}

var tierStories = map[string]tierStory{
	"Badge In":      {"September", "September", "you read the raw lines the dashboards miss"},
	"The Key":       {"October", "October", "you carry the Key, with types and errors that tell the truth"},
	"Hand Tools":    {"November", "November", "tools you built by hand, for the next night"},
	"Surge":         {"Launch Week", "Launch Week", "code that holds when the whole city calls at once"},
	"Dead Air":      {"December to 1 January", "December", "you hear what the silence says"},
	"Forensics":     {"January and February", "January", "you read what the machine wrote, every quote and every column"},
	"Containment":   {"March to May", "Spring", "limits, budgets and expiry that no optimiser can talk its way around"},
	"Blast Radius":  {"June and July", "Summer", "you know what breaks, in what order, and you stop it spreading"},
	"Clean Signal":  {"August to mid-October", "Autumn", "every signal you serve is honest"},
	"The Key Turns": {"To the night the clocks go back", "The last week of October", "your tools held the city's night"},
}

// Tag is the tier's ID on its stamped tag: "T2".
func (t *Tier) Tag() string { return fmt.Sprintf("T%d", t.N) }

// Stamp is the text of the tier's tag: "T2 · THE KEY". The middle dot is
// part of the stamp, so it is the only place a tier label keeps one.
func (t *Tier) Stamp() string {
	if t.Name == "" {
		return t.Tag()
	}
	return t.Tag() + " · " + strings.ToUpper(t.Name)
}

// stampLine is the most runes the ring's tag sets on 1 line: in Saira at
// 13.5 units and 62 % wide, 48 units hold "BADGE IN" and not "HAND TOOLS".
const stampLine = 8

// StampLines is the name as the ring's tag stamps it: 1 line up to
// stampLine runes, else 2 lines at the space that balances them ("HAND",
// "TOOLS"; "THE KEY", "TURNS"). A long name of 1 word stays on 1 line, and
// the drawing sets it narrow. nil for a tier without a name.
func (t *Tier) StampLines() []string {
	if t.Name == "" {
		return nil
	}
	name := strings.ToUpper(t.Name)
	if utf8.RuneCountInString(name) <= stampLine || !strings.Contains(name, " ") {
		return []string{name}
	}
	best, bestLen := 0, -1
	for i, r := range name {
		if r != ' ' {
			continue
		}
		longer := max(utf8.RuneCountInString(name[:i]), utf8.RuneCountInString(name[i+1:]))
		if bestLen < 0 || longer < bestLen {
			best, bestLen = i, longer
		}
	}
	return []string{name[:best], name[best+1:]}
}

// Year is the tier's year in the story: the README makes tiers 1 to 5 the
// first months and tiers 6 to 10 year 2.
func (t *Tier) Year() int { return (t.N-1)/5 + 1 }

// When is the span of months the tier covers in the story: "December to
// 1 January". "" for a tier the story does not know.
func (t *Tier) When() string { return tierStories[t.Name].when }

// Month is the month in Meridian while the tier is open: "December",
// "Spring". "" for a tier the story does not know.
func (t *Tier) Month() string { return tierStories[t.Name].month }

// Line is what a tier certificate says after its colon: "you read the raw
// lines the dashboards miss". "" for a tier the story does not know.
func (t *Tier) Line() string { return tierStories[t.Name].line }

// LineSentence is Line as a sentence of its own: "You read the raw lines
// the dashboards miss." "" for a tier the story does not know.
func (t *Tier) LineSentence() string {
	l := t.Line()
	if l == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(l)
	return strings.ToUpper(string(r)) + l[size:] + "."
}
