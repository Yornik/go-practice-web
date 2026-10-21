// Package course loads the go-practice course from its checkout and keeps
// it in memory: the exercises with their rendered READMEs, test steps and
// files, the hidden hint ladder, and the guide and tiers from the root
// README.
//
// A *Course is read only after Load and safe for concurrent use.
package course

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// GuideSections are the root README sections that the guide page shows,
// in the order of the README.
var GuideSections = []string{
	"Why this exists",
	"Start here",
	"The rules",
	"How to work an exercise",
	"A problem-solving checklist",
	"Make it a tool",
}

// Course is the loaded course.
type Course struct {
	Dir    string        // the checkout Load read
	Commit string        // the checkout's git commit; empty when the directory has no git metadata
	Digest string        // SHA-256 of every file Load read, hex; changes with any content change
	Intro  template.HTML // the root README's paragraph below the title
	Guide  []Section     // the GuideSections that the root README has

	Tiers     []*Tier     // tiers 1 to N, in order
	Exercises []*Exercise // in directory order, which is number order

	// levels describes each level of the hint ladder, from the root
	// README's "Level | What it gives you" table; empty when it has none.
	levels map[Level]string

	bySlug map[string]*Exercise
}

// Tier is a group of exercises from the root README's tier table.
type Tier struct {
	N           int
	Name        string // "Carrying the Owl"; "" when the table has no Name column
	Theme       string // "Warm-up: Go idioms on real data"
	First, Last int    // exercise numbers, inclusive
	Exercises   []*Exercise
}

// Title is "Tier N".
func (t *Tier) Title() string { return fmt.Sprintf("Tier %d", t.N) }

// Label is the title with the tier's name, "Tier 2 · Carrying the Owl",
// or only the title when the tier has no name.
func (t *Tier) Label() string {
	if t.Name == "" {
		return t.Title()
	}
	return t.Title() + " · " + t.Name
}

// Range is the exercise range as the course writes it, "01–03".
func (t *Tier) Range() string { return fmt.Sprintf("%02d–%02d", t.First, t.Last) }

// Hours is the sum of the exercise times.
func (t *Tier) Hours() float64 {
	var h float64
	for _, e := range t.Exercises {
		h += e.Hours
	}
	return h
}

// Version identifies the loaded content: the short commit when known,
// otherwise a short content digest.
func (c *Course) Version() string {
	if len(c.Commit) >= 12 {
		return c.Commit[:12]
	}
	return "sha256-" + c.Digest[:12]
}

// Exercise returns the exercise with the given slug, for example "07-retry".
func (c *Course) Exercise(slug string) (*Exercise, bool) {
	e, ok := c.bySlug[slug]
	return e, ok
}

// ExerciseByNumber returns the exercise with the given number.
func (c *Course) ExerciseByNumber(n int) (*Exercise, bool) {
	i := slices.IndexFunc(c.Exercises, func(e *Exercise) bool { return e.Number == n })
	if i < 0 {
		return nil, false
	}
	return c.Exercises[i], true
}

// Tier returns tier n (1 to len(c.Tiers)).
func (c *Course) Tier(n int) (*Tier, bool) {
	if n < 1 || n > len(c.Tiers) {
		return nil, false
	}
	return c.Tiers[n-1], true
}

// Neighbours returns the exercises before and after slug; nil at the ends.
func (c *Course) Neighbours(slug string) (prev, next *Exercise) {
	i := slices.IndexFunc(c.Exercises, func(e *Exercise) bool { return e.Slug == slug })
	if i < 0 {
		return nil, nil
	}
	if i > 0 {
		prev = c.Exercises[i-1]
	}
	if i+1 < len(c.Exercises) {
		next = c.Exercises[i+1]
	}
	return prev, next
}

// Hours is the time of the whole course.
func (c *Course) Hours() float64 {
	var h float64
	for _, e := range c.Exercises {
		h += e.Hours
	}
	return h
}

var slugPattern = regexp.MustCompile(`^[0-9]{2}-[a-z0-9-]+$`)

// Load reads the course checkout in dir. It fails on any structural problem
// (a README without its tier line, hints that do not split into 3, a
// solution file without a stub) so that a broken course bump stops the
// server at start instead of breaking 1 page later.
func Load(dir string) (*Course, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "exercises"))
	if err != nil {
		return nil, fmt.Errorf("course: %w", err)
	}
	c := &Course{Dir: dir, bySlug: map[string]*Exercise{}}
	var slugs []string
	for _, ent := range entries {
		if ent.IsDir() && slugPattern.MatchString(ent.Name()) {
			slugs = append(slugs, ent.Name())
		}
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("course: no exercises in %s", dir)
	}
	known := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		known[s] = true
	}
	md := newMarkdown(known)

	// Highlighting the READMEs is CPU bound; the exercises load in parallel.
	c.Exercises = make([]*Exercise, len(slugs))
	errs := make([]error, len(slugs))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, slug := range slugs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			c.Exercises[i], errs[i] = loadExercise(md, dir, slug)
			if errs[i] != nil {
				errs[i] = fmt.Errorf("course: %s: %w", slug, errs[i])
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	for _, e := range c.Exercises {
		c.bySlug[e.Slug] = e
	}
	if err := c.loadRoot(md); err != nil {
		return nil, fmt.Errorf("course: README.md: %w", err)
	}
	if c.Digest, err = digest(dir); err != nil {
		return nil, fmt.Errorf("course: %w", err)
	}
	c.Commit = readCommit(dir)
	return c, nil
}

// loadRoot reads the root README: intro, guide sections, tier table and the
// exercise table.
func (c *Course) loadRoot(md *markdown) error {
	src, err := os.ReadFile(filepath.Join(c.Dir, "README.md"))
	if err != nil {
		return err
	}
	doc := md.parse(src, "README.md")
	parts := split(doc)
	var intro []ast.Node
	for _, n := range parts.preamble {
		if h, ok := n.(*ast.Heading); !ok || h.Level != 1 {
			intro = append(intro, n)
		}
	}
	if c.Intro, err = md.renderNodes(doc, intro); err != nil {
		return err
	}
	_, all, err := md.renderSplit(doc, parts)
	if err != nil {
		return err
	}
	for _, title := range GuideSections {
		if i := slices.IndexFunc(all, func(s Section) bool { return s.Title == title }); i >= 0 {
			c.Guide = append(c.Guide, all[i])
		}
	}

	var tierTable, exerciseTable, levelTable *east.Table
	_ = ast.Walk(doc.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		t, ok := n.(*east.Table)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		switch strings.Join(headerCells(t, src), "|") {
		case "Tier|Exercises|Theme", "Tier|Exercises|Name|Theme":
			tierTable = t
		case "#|Exercise|Tier|Time|Go you practice|The real-life skill":
			exerciseTable = t
		case "Level|What it gives you":
			levelTable = t
		}
		return ast.WalkSkipChildren, nil
	})
	if tierTable == nil {
		return fmt.Errorf("no tier table (| Tier | Exercises | Name | Theme |)")
	}
	if err := c.parseTiers(tierTable, src); err != nil {
		return err
	}
	if exerciseTable != nil {
		if err := c.parseExerciseTable(md, doc, exerciseTable); err != nil {
			return err
		}
	}
	if levelTable != nil {
		c.parseLevelTable(levelTable, src)
	}
	return nil
}

// parseLevelTable reads the descriptions of the ladder levels. A row whose
// first cell is not a level name is left out: the table is help text, not
// structure the site depends on.
func (c *Course) parseLevelTable(t *east.Table, src []byte) {
	c.levels = map[Level]string{}
	for _, row := range rows(t) {
		if len(row) != 2 {
			continue
		}
		name := plainText(row[0], src)
		for l := Hint1; l <= MaxLevel; l++ {
			if strings.EqualFold(l.String(), name) {
				c.levels[l] = plainText(row[1], src)
			}
		}
	}
}

// LevelDescription is what a level of the hint ladder gives, as the course
// README says it, for example "A nudge: the question to ask, or the
// concept that fits." It is empty when the README does not say.
func (c *Course) LevelDescription(l Level) string { return c.levels[l] }

func headerCells(t *east.Table, src []byte) []string {
	var cells []string
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		if _, ok := r.(*east.TableHeader); !ok {
			continue
		}
		for cell := r.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, plainText(cell, src))
		}
	}
	return cells
}

func rows(t *east.Table) [][]ast.Node {
	var out [][]ast.Node
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		if _, ok := r.(*east.TableRow); !ok {
			continue
		}
		var cells []ast.Node
		for cell := r.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, cell)
		}
		out = append(out, cells)
	}
	return out
}

var tierRange = regexp.MustCompile(`^([0-9]+)\s*[–-]\s*([0-9]+)$`)

// parseTiers reads the tier table. The course added the Name column with
// its 10 tiers; the 3-column table of the first course still loads.
func (c *Course) parseTiers(t *east.Table, src []byte) error {
	cols := len(headerCells(t, src))
	for _, row := range rows(t) {
		if len(row) != cols {
			return fmt.Errorf("tier table row has %d cells, want %d", len(row), cols)
		}
		n, err := strconv.Atoi(plainText(row[0], src))
		if err != nil || n != len(c.Tiers)+1 {
			return fmt.Errorf("tier table: tier %q is not %d", plainText(row[0], src), len(c.Tiers)+1)
		}
		m := tierRange.FindStringSubmatch(plainText(row[1], src))
		if m == nil {
			return fmt.Errorf("tier table: tier %d: range %q", n, plainText(row[1], src))
		}
		tier := &Tier{N: n, Theme: plainText(row[cols-1], src)}
		if cols == 4 {
			tier.Name = plainText(row[2], src)
		}
		// GFM pads a short row with empty cells; a tier without its theme
		// or name is a broken table, not a tier.
		if tier.Theme == "" || cols == 4 && tier.Name == "" {
			return fmt.Errorf("tier table: tier %d has an empty cell", n)
		}
		tier.First, _ = strconv.Atoi(m[1])
		tier.Last, _ = strconv.Atoi(m[2])
		c.Tiers = append(c.Tiers, tier)
	}
	for _, e := range c.Exercises {
		i := slices.IndexFunc(c.Tiers, func(t *Tier) bool { return e.Number >= t.First && e.Number <= t.Last })
		if i < 0 {
			return fmt.Errorf("exercise %s is in no tier of the tier table", e.Slug)
		}
		tier := c.Tiers[i]
		if tier.N != e.Tier {
			return fmt.Errorf("exercise %s: its README says tier %d, the tier table says %d", e.Slug, e.Tier, tier.N)
		}
		tier.Exercises = append(tier.Exercises, e)
	}
	for _, t := range c.Tiers {
		if len(t.Exercises) != t.Last-t.First+1 {
			return fmt.Errorf("tier %d (%s) has %d exercises", t.N, t.Range(), len(t.Exercises))
		}
	}
	return nil
}

// parseExerciseTable takes the 2 columns that only the root README has.
func (c *Course) parseExerciseTable(md *markdown, doc document, t *east.Table) error {
	for _, row := range rows(t) {
		if len(row) != 6 {
			return fmt.Errorf("exercise table row has %d cells, want 6", len(row))
		}
		n, err := strconv.Atoi(plainText(row[0], doc.src))
		if err != nil {
			return fmt.Errorf("exercise table: number %q", plainText(row[0], doc.src))
		}
		e, ok := c.ExerciseByNumber(n)
		if !ok {
			continue
		}
		e.table = tableRow{
			title: plainText(row[1], doc.src),
			tier:  plainText(row[2], doc.src),
			time:  plainText(row[3], doc.src),
		}
		if e.Practice, err = md.renderNodes(doc, children(row[4])); err != nil {
			return err
		}
		if e.Skill, err = md.renderNodes(doc, children(row[5])); err != nil {
			return err
		}
	}
	return nil
}

func children(n ast.Node) []ast.Node {
	var out []ast.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, c)
	}
	return out
}

// digest hashes the files Load reads, in a fixed order.
func digest(dir string) (string, error) {
	h := sha256.New()
	var paths []string
	for _, pattern := range []string{"README.md", "exercises/*/*", "exercises/*/testdata/*", "solutions/*/*"} {
		m, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return "", err
		}
		paths = append(paths, m...)
	}
	slices.Sort(paths)
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		if fi.IsDir() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(dir, p)
		// hash.Hash writes never fail.
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		_, _ = h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var commitHash = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// readCommit resolves HEAD of the checkout without running git: the web
// image has no git binary. It handles a .git directory, a .git file (a
// submodule or worktree) and packed refs. It returns "" when anything is
// missing.
func readCommit(dir string) string {
	gitDir := filepath.Join(dir, ".git")
	fi, err := os.Stat(gitDir)
	if err != nil {
		return ""
	}
	if !fi.IsDir() {
		b, err := os.ReadFile(gitDir)
		if err != nil {
			return ""
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		gitDir = target
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	ref, isRef := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: ")
	if !isRef {
		return validCommit(ref)
	}
	// A worktree keeps shared refs in the common directory.
	dirs := []string{gitDir}
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common := strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		dirs = append(dirs, common)
	}
	for _, d := range dirs {
		if b, err := os.ReadFile(filepath.Join(d, filepath.FromSlash(ref))); err == nil {
			return validCommit(strings.TrimSpace(string(b)))
		}
		if b, err := os.ReadFile(filepath.Join(d, "packed-refs")); err == nil {
			for line := range bytes.Lines(b) {
				hash, name, ok := strings.Cut(strings.TrimSpace(string(line)), " ")
				if ok && name == ref {
					return validCommit(hash)
				}
			}
		}
	}
	return ""
}

func validCommit(s string) string {
	if commitHash.MatchString(s) {
		return s
	}
	return ""
}
