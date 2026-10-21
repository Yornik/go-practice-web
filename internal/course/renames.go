package course

import "maps"

// renames maps the slugs of the first course (25 exercises in 5 tiers) to
// their slugs in the course of 50 exercises, where they are tiers 6 to 10.
// Stored progress, code, reveals and signed certificates still name the
// old slugs, so every lookup that takes a slug from outside the course
// goes through this table. It is the only place that names the new slugs
// of the renamed exercises.
var renames = map[string]string{
	"01-logfmt":         "26-logfmt",
	"02-accesslog":      "27-accesslog",
	"03-latency":        "28-latency",
	"04-grep":           "29-grep",
	"05-slo":            "30-slo",
	"06-envconfig":      "31-envconfig",
	"07-retry":          "32-retry",
	"08-ratelimit":      "33-ratelimit",
	"09-cache":          "34-cache",
	"10-deployorder":    "35-deployorder",
	"11-tracetree":      "36-tracetree",
	"12-workerpool":     "37-workerpool",
	"13-logmerge":       "38-logmerge",
	"14-circuitbreaker": "39-circuitbreaker",
	"15-healthcheck":    "40-healthcheck",
	"16-middleware":     "41-middleware",
	"17-promtext":       "42-promtext",
	"18-certcheck":      "43-certcheck",
	"19-alertgroup":     "44-alertgroup",
	"20-singleflight":   "45-singleflight",
	"21-broadcaster":    "46-broadcaster",
	"22-reconciler":     "47-reconciler",
	"23-leaderelection": "48-leaderelection",
	"24-diskbuffer":     "49-diskbuffer",
	"25-loadbalancer":   "50-loadbalancer",
}

// Renames returns a copy of the rename table: old slug to new slug. The
// data migration and the tests use it; lookups use Resolve.
func Renames() map[string]string {
	return maps.Clone(renames)
}

// Renamed returns the new slug of an old one, whatever course is loaded.
func Renamed(old string) (string, bool) {
	s, ok := renames[old]
	return s, ok
}

// Resolve returns the exercise for slug: the exercise with that slug, or,
// when the course has none, the exercise an old slug was renamed to. renamed
// is true in the second case. With the first course loaded the old slugs
// are current, so nothing resolves through the table.
func (c *Course) Resolve(slug string) (e *Exercise, renamed, ok bool) {
	if e, ok := c.bySlug[slug]; ok {
		return e, false, true
	}
	if to, found := renames[slug]; found {
		if e, ok := c.bySlug[to]; ok {
			return e, true, true
		}
	}
	return nil, false, false
}

// CurrentSlug returns the slug the course uses for slug: itself when the
// course has it, its new slug when it was renamed, and "" when the course
// knows neither.
func (c *Course) CurrentSlug(slug string) string {
	e, _, ok := c.Resolve(slug)
	if !ok {
		return ""
	}
	return e.Slug
}
