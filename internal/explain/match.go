package explain

import (
	"html"
	"maps"
	"slices"
	"strings"
)

// finding is one message to explain, from a diagnostic or from test output.
type finding struct {
	source string // the catalog source to search first
	code   string // vet analyzer or staticcheck code, if known
	guide  string // the fallback guide
	raw    Raw
	// text is what match patterns run against; empty means Raw.Message.
	text     string
	builtins map[string]string
}

// pools lists where to look for each source. Compile and build errors
// overlap (an import problem can come from either), so each also searches
// the other after its own entries.
var pools = map[string][]string{
	SourceCompile:     {SourceCompile, SourceBuild},
	SourceBuild:       {SourceBuild, SourceCompile},
	SourceVet:         {SourceVet},
	SourceStaticcheck: {SourceStaticcheck},
	SourceRuntime:     {SourceRuntime},
	SourceRace:        {SourceRace},
	SourceTest:        {SourceTest},
}

// Ranks of a match, from the most to the least specific. When 2 entries
// match with the same rank, the first in file order wins; keep patterns
// disjoint so that this never decides.
const (
	rankNone = iota
	rankCategory
	rankCategoryPattern
	rankFallbackPattern // a pattern of an entry marked fallback
	rankPattern
	rankCode
	rankCodePattern
)

func (c *catalogData) explain(f finding) Explanation {
	f.raw.Message = strings.TrimRight(f.raw.Message, " \t\n")
	text := f.raw.Message
	if f.text != "" {
		text = strings.TrimRight(f.text, " \t\n")
	}
	if c != nil {
		for _, src := range pools[f.source] {
			var best *entry
			var bestGroups map[string]string
			bestRank := rankNone
			for _, e := range c.bySource[src] {
				rank, groups := e.match(f.code, text)
				if rank > bestRank {
					best, bestRank, bestGroups = e, rank, groups
				}
			}
			if best != nil {
				values := maps.Clone(f.builtins)
				if values == nil {
					values = make(map[string]string)
				}
				for k, v := range bestGroups {
					if v != "" || values[k] == "" {
						values[k] = v
					}
				}
				return best.explanation(values, f.raw, false)
			}
		}
		if g, ok := c.byID[f.guide]; ok {
			return g.explanation(f.builtins, f.raw, true)
		}
	}
	// Only a broken catalog gets here; Check reports why. The learner still
	// sees the message itself.
	return Explanation{
		ID:      "unknown",
		Source:  SourceGuide,
		Title:   "No explanation available",
		HTML:    "<p>Read the message from left to right: file, line, column, then what Go found.</p>\n",
		Links:   []Link{},
		Generic: true,
		Raw:     withCaptures(f.raw, f.builtins),
	}
}

// match reports how well the entry fits a message with the given code, and
// the named groups of the pattern that matched.
func (e *entry) match(code, msg string) (int, map[string]string) {
	rank := rankPattern
	if e.code != "" {
		switch {
		case code == "":
			// The code is unknown (go vet's text output inside go test), so
			// only a pattern can decide.
			if len(e.patterns) == 0 {
				return rankNone, nil
			}
		case code == e.code:
			rank = rankCode
		case e.category && strings.HasPrefix(code, e.code):
			rank = rankCategory
		default:
			return rankNone, nil
		}
	}
	if len(e.patterns) == 0 {
		return rank, nil
	}
	for _, re := range e.patterns {
		m := re.FindStringSubmatchIndex(msg)
		if m == nil {
			continue
		}
		groups := make(map[string]string)
		for i, name := range re.SubexpNames() {
			// With alternation the same name can appear twice; keep the
			// group that took part in the match.
			if name == "" || m[2*i] < 0 || groups[name] != "" {
				continue
			}
			groups[name] = msg[m[2*i]:m[2*i+1]]
		}
		switch rank {
		case rankCode:
			return rankCodePattern, groups
		case rankCategory:
			return rankCategoryPattern, groups
		default:
			if e.fallback {
				return rankFallbackPattern, groups
			}
			return rankPattern, groups
		}
	}
	return rankNone, nil
}

// explanation fills in the entry for 1 message.
func (e *entry) explanation(values map[string]string, raw Raw, generic bool) Explanation {
	// A copy, so a caller that edits the result cannot change the catalog.
	links := slices.Clone(e.links)
	if links == nil {
		links = []Link{}
	}
	return Explanation{
		ID:      e.id,
		Source:  e.source,
		Title:   e.title.fill(values, titleValue),
		HTML:    e.html.fill(values, html.EscapeString),
		Links:   links,
		Course:  e.course,
		Generic: generic,
		Raw:     withCaptures(raw, values),
	}
}

func withCaptures(raw Raw, values map[string]string) Raw {
	for k, v := range values {
		if v == "" {
			continue
		}
		if raw.Captures == nil {
			raw.Captures = make(map[string]string)
		}
		raw.Captures[k] = v
	}
	return raw
}
