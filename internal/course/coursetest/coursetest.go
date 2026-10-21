// Package coursetest writes small synthetic courses for tests: the first
// course's shape (25 exercises, 5 tiers, a 3-column tier table) and the
// course of 50 exercises in 10 named tiers, in which the first course's
// exercises are tiers 6 to 10 under their new slugs. Each exercise has 1
// stub, 1 test file, 2 steps and its hidden material, so course.Load reads
// it like the real course, in a fraction of the time.
package coursetest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/course"
)

// Exercise is 1 exercise of a fixture.
type Exercise struct {
	Slug  string // "07-retry"
	Title string
	Hours float64
}

// Tier is 1 tier of a fixture.
type Tier struct {
	Name      string // "" in the first course
	Theme     string
	Exercises []Exercise
}

// Spec describes a fixture course.
type Spec struct {
	Named bool // the tier table has the Name column
	Tiers []Tier
}

// firstCourse is the first course: slugs, titles and hours as it shipped
// them, tiers of 3, 6, 5, 5 and 6 exercises.
var firstCourse = []struct {
	theme string
	exs   []Exercise
}{
	{"Warm-up: Go idioms on real data", []Exercise{
		{"01-logfmt", "Parse logfmt log lines", 1.5},
		{"02-accesslog", "Find the endpoints that hurt", 1},
		{"03-latency", "Percentiles and histogram quantiles", 1.5},
	}},
	{"Streams, state, time, types, errors and locks", []Exercise{
		{"04-grep", "Streaming log filter with context", 2.5},
		{"05-slo", "Error budgets and burn-rate alerts", 1.5},
		{"06-envconfig", "Typed config from the environment", 2.5},
		{"07-retry", "Retries with backoff, jitter and context", 2.5},
		{"08-ratelimit", "Token bucket and per-client limits", 2},
		{"09-cache", "Cache: generic LRU with TTL", 2},
	}},
	{"Algorithms on real data, your own goroutines", []Exercise{
		{"10-deployorder", "Deploy order, waves and blast radius", 2.5},
		{"11-tracetree", "Make sense of a distributed trace", 2.5},
		{"12-workerpool", "Worker pool: bounded parallelism, ordered results, fail fast", 2.5},
		{"13-logmerge", "Merge logs from many pods in time order", 2.5},
		{"14-circuitbreaker", "Circuit breaker: stop hammering a dead dependency", 3.5},
	}},
	{"Services, protocols and alerting logic", []Exercise{
		{"15-healthcheck", "Health checks: /livez, /readyz and graceful shutdown", 3},
		{"16-middleware", "HTTP middleware that operators need", 3},
		{"17-promtext", "Parse Prometheus metrics and compute rates", 4.5},
		{"18-certcheck", "TLS certificate expiry checker", 3},
		{"19-alertgroup", "Alert dedup, grouping and inhibition", 4},
	}},
	{"Distributed-systems building blocks", []Exercise{
		{"20-singleflight", "Singleflight: stop the cache stampede", 3},
		{"21-broadcaster", "Broadcaster: watch streams with slow consumers", 3.5},
		{"22-reconciler", "A Kubernetes-style controller", 4},
		{"23-leaderelection", "Leader election: leases and fencing tokens", 4},
		{"24-diskbuffer", "Crash-safe disk buffer for a log shipper", 4.5},
		{"25-loadbalancer", "Capstone: a health-aware load balancer", 6},
	}},
}

// newTiers are tiers 1 to 5 of the course of 50, with made-up exercises:
// a fixture needs their shape, not their content.
var newTiers = []Tier{
	{"First week", "Go basics on small real tasks: strings, maps, slices and sorting", []Exercise{
		{"01-loglevels", "Count the errors the dashboard missed", 1},
		{"02-repeated", "Fold the repeated lines", 1},
		{"03-pagenoise", "Which alert pages the most", 1},
		{"04-batches", "Split the rollout into batches", 1},
		{"05-envfile", "Read an env file", 1.5},
	}},
	{"Carrying the Owl", "Types with methods, errors you can inspect, enums, time values, text as bytes", []Exercise{
		{"06-semver", "Sort the firmware versions", 1},
		{"07-inventory", "The feeder inventory", 1.5},
		{"08-deploynote", "Deploy notes that fit", 1},
		{"09-statuspage", "The status page", 1.5},
		{"10-oncall", "Who has the Owl", 1.5},
	}},
	{"Your own tools", "Readers and writers, closures, float math, binary search, a testable command", []Exercise{
		{"11-nightlog", "The night log", 1.5},
		{"12-linefilter", "A line filter", 1.5},
		{"13-nines", "Count the nines", 1},
		{"14-bisect", "Bisect the bad deploy", 1.5},
		{"15-checkdisk", "A disk check you can test", 1.5},
	}},
	{"Black Friday", "Interfaces the standard library calls, generics, reflect, goroutines and a mutex", []Exercise{
		{"16-redact", "Secrets that refuse to print", 1},
		{"17-flightrec", "A flight recorder", 1.5},
		{"18-tableprint", "Print any table", 1.5},
		{"19-fanout", "Ask every feeder at once", 1.5},
		{"20-counters", "Counters that add up", 1},
	}},
	{"The long nights", "context, an injected clock, eviction, and hand-written scanners with typed errors", []Exercise{
		{"21-rollout", "A rollout that stops in time", 1.5},
		{"22-alertstate", "Alert states", 1.5},
		{"23-silences", "Silences that expire", 1.5},
		{"24-durations", "Read a duration", 1.5},
		{"25-syslog", "Read the feeders' syslog", 1.5},
	}},
}

// renamedNames are the names of tiers 6 to 10 in the course of 50.
var renamedNames = []string{"The morning after", "Growing pains", "Spring launch", "Dashboards that lie", "Passing the Owl"}

// First is the first course: 25 exercises in 5 tiers without names.
func First() Spec {
	s := Spec{}
	for _, t := range firstCourse {
		s.Tiers = append(s.Tiers, Tier{Theme: t.theme, Exercises: slices.Clone(t.exs)})
	}
	return s
}

// Fifty is the course of 50 exercises in 10 named tiers. Tiers 6 to 10
// hold the first course's exercises under the slugs of course.Renames.
func Fifty() Spec {
	s := Spec{Named: true, Tiers: slices.Clone(newTiers)}
	renames := course.Renames()
	for i, t := range firstCourse {
		tier := Tier{Name: renamedNames[i], Theme: t.theme}
		for _, e := range t.exs {
			e.Slug = renames[e.Slug]
			tier.Exercises = append(tier.Exercises, e)
		}
		s.Tiers = append(s.Tiers, tier)
	}
	return s
}

// Load writes the spec to a temporary directory and loads it.
func Load(tb testing.TB, s Spec) *course.Course {
	tb.Helper()
	dir := tb.TempDir()
	if err := Write(dir, s); err != nil {
		tb.Fatal(err)
	}
	c, err := course.Load(dir)
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// Write writes the spec as a course checkout in dir.
func Write(dir string, s Spec) error {
	var root strings.Builder
	root.WriteString("# Go practice\n\nA fixture course.\n\n## Tiers\n\n")
	if s.Named {
		root.WriteString("| Tier | Exercises | Name | Theme |\n|---|---|---|---|\n")
	} else {
		root.WriteString("| Tier | Exercises | Theme |\n|---|---|---|\n")
	}
	for i, t := range s.Tiers {
		if len(t.Exercises) == 0 {
			return fmt.Errorf("tier %d has no exercises", i+1)
		}
		first, last := number(t.Exercises[0].Slug), number(t.Exercises[len(t.Exercises)-1].Slug)
		if s.Named {
			fmt.Fprintf(&root, "| %d | %02d–%02d | %s | %s |\n", i+1, first, last, t.Name, t.Theme)
		} else {
			fmt.Fprintf(&root, "| %d | %02d–%02d | %s |\n", i+1, first, last, t.Theme)
		}
		for _, e := range t.Exercises {
			if err := writeExercise(dir, i+1, e); err != nil {
				return err
			}
		}
	}
	if err := write(filepath.Join(dir, "go.mod"), "module example.com\n\ngo 1.24\n"); err != nil {
		return err
	}
	return write(filepath.Join(dir, "README.md"), root.String())
}

func number(slug string) int {
	n, _ := strconv.Atoi(slug[:2])
	return n
}

func writeExercise(dir string, tier int, e Exercise) error {
	pkg := strings.ReplaceAll(e.Slug[3:], "-", "")
	hours := strconv.FormatFloat(e.Hours, 'f', -1, 64)
	readme := fmt.Sprintf("# %02d · %s\n\n**Tier %d · about %s h** · Go: maps · Problem solving: read the spec\n\n"+
		"The scenario.\n\n## Task\n\nWrite `Run`.\n\n## Run the tests\n\n```bash\n"+
		"go test -run TestRun -v ./exercises/%[5]s/   # step 1: Run\n"+
		"go test -race -run TestMore -v ./exercises/%[5]s/   # step 2: more\n```\n",
		number(e.Slug), e.Title, tier, hours, e.Slug)
	stub := "package " + pkg + "\n\n// Run is the stub.\nfunc Run() int {\n\tpanic(\"TODO: implement\")\n}\n"
	test := "package " + pkg + "_test\n\nimport (\n\t\"testing\"\n\n\t" + pkg + " \"example.com/exercises/" + e.Slug + "\"\n)\n\n" +
		"func TestRun(t *testing.T) { _ = " + pkg + ".Run() }\n\nfunc TestMore(t *testing.T) { _ = " + pkg + ".Run() }\n"
	solution := "package " + pkg + "\n\n// Run is the solution.\nfunc Run() int { return 1 }\n"
	files := map[string]string{
		filepath.Join("exercises", e.Slug, "README.md"):             readme,
		filepath.Join("exercises", e.Slug, pkg+".go"):               stub,
		filepath.Join("exercises", e.Slug, pkg+"_test.go"):          test,
		filepath.Join("solutions", e.Slug, "hints.md.rot13"):        rot13("# Hints\n\n## Hint 1\n\nOne.\n\n## Hint 2\n\nTwo.\n\n## Hint 3\n\nThree.\n"),
		filepath.Join("solutions", e.Slug, "explanation.md.rot13"):  rot13("# Explanation\n\nWhy.\n"),
		filepath.Join("solutions", e.Slug, "walkthrough.md.rot13"):  rot13("# Walkthrough\n\nHow.\n"),
		filepath.Join("solutions", e.Slug, pkg+".go.rot13"):         rot13(solution),
		filepath.Join("exercises", e.Slug, "testdata", "input.txt"): "level=info\n",
	}
	for name, content := range files {
		if err := write(filepath.Join(dir, name), content); err != nil {
			return err
		}
	}
	return nil
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func rot13(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		}
	}
	return string(b)
}
