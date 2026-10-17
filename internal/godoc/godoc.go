// Package godoc serves standard library documentation to the editor: hover text, completion after "pkg." and
// signature help. The course allows go doc and pkg.go.dev, so the editor offers the same text in place.
//
// cmd/gendocs builds the index from the GOROOT of the runner image's Go release, so the docs describe the
// compiler that runs the learner's code. Regenerate it when the runner image moves to a new Go release:
//
//	GOTOOLCHAIN=go1.27.2 go generate ./internal/godoc
package godoc

//go:generate go run ../../cmd/gendocs -go go1.27 -o data/stdlib.json.gz

import (
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
)

// FormatVersion is the version of the index file layout. Load rejects other versions, so an old binary never
// misreads a new index.
const FormatVersion = 1

// pkgsite is where every package and member links to, because the full docs and the examples live there.
const pkgsite = "https://pkg.go.dev/"

// maxEmbedDepth bounds the search through embedded types. Struct embedding can form cycles through pointers.
const maxEmbedDepth = 4

// Kind says what a Member declares.
type Kind string

// The kinds of members. A method of an interface type is a KindMethod too.
const (
	KindFunc   Kind = "func"
	KindType   Kind = "type"
	KindMethod Kind = "method"
	KindConst  Kind = "const"
	KindVar    Kind = "var"
	KindField  Kind = "field"
)

func (k Kind) valid() bool {
	switch k {
	case KindFunc, KindType, KindMethod, KindConst, KindVar, KindField:
		return true
	}
	return false
}

// Index is the documentation of the standard library of 1 Go release for 1 platform.
// It is read-only after Load and safe for concurrent use.
type Index struct {
	Format    int        `json:"format"`
	GoVersion string     `json:"goVersion"`
	GOOS      string     `json:"goos"`
	GOARCH    string     `json:"goarch"`
	Packages  []*Package `json:"packages"`

	byPath map[string]*Package
	etag   string
}

// Package is the documentation of 1 package.
type Package struct {
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Synopsis   string   `json:"synopsis,omitempty"`
	Doc        string   `json:"doc,omitempty"`
	Deprecated string   `json:"deprecated,omitempty"`
	Members    []Member `json:"members"`

	ix *Index
	// nTop is the number of package-level members. They come first in Members, sorted by name, and the methods
	// and fields follow, sorted by receiver and name. Binary search on that order needs no extra memory.
	nTop int
}

// Member is 1 exported identifier: a package-level func, type, const or var, or a method or field of a type.
type Member struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	// Recv is the name of the type that owns a method or field, without "*" or type parameters.
	Recv string `json:"recv,omitempty"`
	// Signature is the declaration as gofmt prints it, on 1 line where that stays readable.
	Signature string `json:"signature"`
	// Doc is the first paragraph of the doc comment as plain text.
	Doc string `json:"doc,omitempty"`
	// Deprecated is the text after "Deprecated:" in the doc comment.
	Deprecated string `json:"deprecated,omitempty"`
	// Embeds lists the embedded types of a struct or interface type: "Reader" for a type of the same package,
	// "io.Reader" (import path, a dot, the type name) for another package. Method follows them to find
	// promoted methods and fields.
	Embeds []string `json:"embeds,omitempty"`

	pkg  *Package
	kids []Member // methods and fields of a type, sorted by name
}

// Load reads an index written by Encode.
func Load(r io.Reader) (*Index, error) {
	h := sha256.New()
	zr, err := gzip.NewReader(io.TeeReader(r, h))
	if err != nil {
		return nil, fmt.Errorf("godoc: read index: %w", err)
	}
	var ix Index
	if err := json.NewDecoder(zr).Decode(&ix); err != nil {
		return nil, fmt.Errorf("godoc: decode index: %w", err)
	}
	// Reading to the end checks the gzip checksum and feeds every byte to the hash.
	if _, err := io.Copy(io.Discard, zr); err != nil {
		return nil, fmt.Errorf("godoc: read index: %w", err)
	}
	if ix.Format != FormatVersion {
		return nil, fmt.Errorf("godoc: index format %d, want %d; regenerate it with cmd/gendocs", ix.Format, FormatVersion)
	}
	ix.etag = fmt.Sprintf(`"%s-%x"`, ix.GoVersion, h.Sum(nil)[:8])
	if err := ix.build(); err != nil {
		return nil, err
	}
	return &ix, nil
}

// LoadFile reads an index from a file written by Encode.
func LoadFile(name string) (*Index, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("godoc: %w", err)
	}
	// Nothing was written, so a Close error cannot lose data.
	defer func() { _ = f.Close() }()
	return Load(f)
}

// The whole directory is embedded so that the package still compiles when the index file is missing.
// cmd/gendocs imports this package, so a hard dependency on the file would stop it from creating the file.
//
//go:embed data
var data embed.FS

const embeddedIndex = "data/stdlib.json.gz"

var loadDefault = sync.OnceValues(func() (*Index, error) {
	f, err := data.Open(embeddedIndex)
	if err != nil {
		return nil, fmt.Errorf("godoc: no embedded index; run go generate ./internal/godoc: %w", err)
	}
	defer func() { _ = f.Close() }() // closing an embedded file cannot fail
	return Load(f)
})

// Default returns the index embedded in the binary. It is loaded on the first call.
func Default() (*Index, error) {
	return loadDefault()
}

// Encode writes ix as gzip-compressed JSON. It sorts the packages and members of ix in place, so the same input
// always gives the same bytes and a regenerated index only changes in git when the docs change.
func Encode(w io.Writer, ix *Index) error {
	ix.Format = FormatVersion
	slices.SortFunc(ix.Packages, func(a, b *Package) int { return strings.Compare(a.Path, b.Path) })
	for _, p := range ix.Packages {
		sortMembers(p.Members)
	}
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(zw)
	// The file is never served as HTML; escaping "<-chan" and "&" only costs bytes.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ix); err != nil {
		return err
	}
	return zw.Close()
}

// sortMembers puts package-level members first, then methods and fields grouped by their type.
func sortMembers(ms []Member) {
	slices.SortStableFunc(ms, func(a, b Member) int {
		if c := strings.Compare(a.Recv, b.Recv); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func (ix *Index) build() error {
	ix.byPath = make(map[string]*Package, len(ix.Packages))
	for _, p := range ix.Packages {
		if p == nil || p.Path == "" {
			return errors.New("godoc: index has a package without a path")
		}
		if _, dup := ix.byPath[p.Path]; dup {
			return fmt.Errorf("godoc: index has package %s twice", p.Path)
		}
		ix.byPath[p.Path] = p
		if err := p.build(ix); err != nil {
			return err
		}
	}
	slices.SortFunc(ix.Packages, func(a, b *Package) int { return strings.Compare(a.Path, b.Path) })
	return nil
}

func (p *Package) build(ix *Index) error {
	p.ix = ix
	sortMembers(p.Members)
	p.nTop = 0
	for i := range p.Members {
		m := &p.Members[i]
		if m.Name == "" || !m.Kind.valid() {
			return fmt.Errorf("godoc: %s: member %q has kind %q", p.Path, m.Name, m.Kind)
		}
		if i > 0 && p.Members[i-1].Recv == m.Recv && p.Members[i-1].Name == m.Name {
			return fmt.Errorf("godoc: %s: member %s is listed twice", p.Path, m.qualifiedName())
		}
		m.pkg = p
		if m.Recv == "" {
			p.nTop++
		}
	}
	for start := p.nTop; start < len(p.Members); {
		recv := p.Members[start].Recv
		end := start + 1
		for end < len(p.Members) && p.Members[end].Recv == recv {
			end++
		}
		t := p.typ(recv)
		if t == nil {
			return fmt.Errorf("godoc: %s: %s has methods or fields but no type", p.Path, recv)
		}
		t.kids = p.Members[start:end:end]
		start = end
	}
	return nil
}

// ETag identifies the index content: the Go version and a hash of the file. HTTP responses use it as their
// validator, so browsers revalidate after a deploy with a new index.
func (ix *Index) ETag() string { return ix.etag }

// Package returns the package with the import path, or nil.
func (ix *Index) Package(path string) *Package {
	return ix.byPath[path]
}

// Member returns a package-level member, or nil. A name of the form "Type.Name" returns a method or field like
// Method does.
func (ix *Index) Member(pkgPath, name string) *Member {
	p := ix.byPath[pkgPath]
	if p == nil {
		return nil
	}
	return p.Member(name)
}

// Method returns the method or field name of the type typeName, or nil. It follows embedded types like the
// compiler does: the shallowest match wins, and 2 matches at the same depth are ambiguous, so the result is nil.
func (ix *Index) Method(pkgPath, typeName, name string) *Member {
	p := ix.byPath[pkgPath]
	if p == nil {
		return nil
	}
	return p.Method(typeName, name)
}

// Complete returns the members of a package whose names start with prefix, best match first. See
// Package.Complete for the order.
func (ix *Index) Complete(pkgPath, prefix string) []*Member {
	p := ix.byPath[pkgPath]
	if p == nil {
		return nil
	}
	return p.Complete(prefix)
}

// CompletePackages returns the packages whose import path or package name starts with prefix: an exact path or name
// first, then path prefixes, then name prefixes ("rand" finds crypto/rand and math/rand/v2). Deprecated packages
// come after the others in each group.
func (ix *Index) CompletePackages(prefix string) []*Package {
	type pkgCand struct {
		p    *Package
		rank int
	}
	var cs []pkgCand
	for _, p := range ix.Packages {
		switch {
		case p.Path == prefix || p.Name == prefix:
			cs = append(cs, pkgCand{p, 0})
		case strings.HasPrefix(p.Path, prefix):
			cs = append(cs, pkgCand{p, 1})
		case strings.HasPrefix(p.Name, prefix):
			cs = append(cs, pkgCand{p, 2})
		}
	}
	slices.SortFunc(cs, func(a, b pkgCand) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if da, db := a.p.Deprecated != "", b.p.Deprecated != ""; da != db {
			if da {
				return 1
			}
			return -1
		}
		return strings.Compare(a.p.Path, b.p.Path)
	})
	out := make([]*Package, len(cs))
	for i, c := range cs {
		out[i] = c.p
	}
	return out
}

// URL links to the package on pkg.go.dev.
func (p *Package) URL() string { return pkgsite + p.Path }

// Member returns a package-level member, or a method or field for "Type.Name", or nil.
func (p *Package) Member(name string) *Member {
	if typ, sel, ok := strings.Cut(name, "."); ok {
		return p.Method(typ, sel)
	}
	return search(p.Members[:p.nTop], name)
}

// Method returns a method or field of a type of this package, or nil. See Index.Method.
func (p *Package) Method(typeName, name string) *Member {
	t := p.typ(typeName)
	if t == nil {
		return nil
	}
	for depth := 0; depth <= maxEmbedDepth; depth++ {
		m, n, reached := t.findAt(name, depth)
		switch {
		case n == 1:
			return m
		case n > 1:
			return nil // ambiguous selector: the compiler rejects it too
		case !reached:
			return nil
		}
	}
	return nil
}

// Complete returns the package-level members whose names start with prefix. A prefix of the form "Type.Pre"
// completes the methods and fields of Type, promoted ones included.
//
// The order: an exact match, then case-sensitive prefix matches, then case-insensitive ones (so "cu" still finds
// Cut). Within each group deprecated members come last, then the order is by name.
func (p *Package) Complete(prefix string) []*Member {
	var cs []cand
	if typ, sel, ok := strings.Cut(prefix, "."); ok {
		t := p.typ(typ)
		if t == nil {
			return nil
		}
		for _, m := range t.methodSet() {
			cs = appendMatch(cs, m, sel)
		}
	} else {
		for i := range p.Members[:p.nTop] {
			cs = appendMatch(cs, &p.Members[i], prefix)
		}
	}
	return sortCands(cs)
}

type cand struct {
	m    *Member
	rank int
}

func appendMatch(cs []cand, m *Member, prefix string) []cand {
	switch {
	case m.Name == prefix:
		return append(cs, cand{m, 0})
	case strings.HasPrefix(m.Name, prefix):
		return append(cs, cand{m, 1})
	case len(m.Name) >= len(prefix) && strings.EqualFold(m.Name[:len(prefix)], prefix):
		return append(cs, cand{m, 2})
	}
	return cs
}

func sortCands(cs []cand) []*Member {
	if len(cs) == 0 {
		return nil
	}
	slices.SortFunc(cs, func(a, b cand) int {
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if da, db := a.m.Deprecated != "", b.m.Deprecated != ""; da != db {
			if da {
				return 1
			}
			return -1
		}
		return strings.Compare(a.m.Name, b.m.Name)
	})
	out := make([]*Member, len(cs))
	for i, c := range cs {
		out[i] = c.m
	}
	return out
}

// typ returns the package-level type with the name, or nil.
func (p *Package) typ(name string) *Member {
	m := search(p.Members[:p.nTop], name)
	if m == nil || m.Kind != KindType {
		return nil
	}
	return m
}

// search is a hand-written binary search: a closure for sort.Search would escape and allocate on every lookup.
func search(ms []Member, name string) *Member {
	lo, hi := 0, len(ms)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if ms[mid].Name < name {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(ms) && ms[lo].Name == name {
		return &ms[lo]
	}
	return nil
}

// Package returns the package that declares m.
func (m *Member) Package() *Package { return m.pkg }

// URL links to the member on pkg.go.dev: #Name, or #Type.Name for a method or field.
func (m *Member) URL() string {
	return pkgsite + m.pkg.Path + "#" + m.qualifiedName()
}

func (m *Member) qualifiedName() string {
	if m.Recv != "" {
		return m.Recv + "." + m.Name
	}
	return m.Name
}

// embedded resolves 1 entry of Embeds to the type it names, or nil when the index does not have it.
func (m *Member) embedded(e string) *Member {
	p := m.pkg
	if i := strings.LastIndexByte(e, '.'); i >= 0 {
		p = p.ix.byPath[e[:i]]
		e = e[i+1:]
	}
	if p == nil {
		return nil
	}
	return p.typ(e)
}

// findAt looks for name exactly depth embedding levels below type t. It returns the match, the number of
// distinct matches, and whether any type sits at that depth. When none does, deeper levels cannot match either.
func (t *Member) findAt(name string, depth int) (found *Member, n int, reached bool) {
	if depth == 0 {
		if m := search(t.kids, name); m != nil {
			return m, 1, true
		}
		return nil, 0, true
	}
	for _, e := range t.Embeds {
		et := t.embedded(e)
		if et == nil {
			continue
		}
		m, k, r := et.findAt(name, depth-1)
		reached = reached || r
		if k == 0 {
			continue
		}
		// An interface may embed 2 interfaces that share a method; that is 1 method, not an ambiguity.
		if found == nil {
			found, n = m, k
		} else if m != found || k > 1 {
			n += k
		}
	}
	return found, n, reached
}

// methodSet returns the methods and fields reachable from type t, promoted ones included. Ambiguous names are
// left out, as the compiler would reject them.
func (t *Member) methodSet() []*Member {
	seen := make(map[string]bool) // names settled at a shallower depth
	var out []*Member
	level := []*Member{t}
	for depth := 0; depth <= maxEmbedDepth && len(level) > 0; depth++ {
		found := make(map[string]*Member)
		ambiguous := make(map[string]bool)
		var next []*Member
		for _, lt := range level {
			for i := range lt.kids {
				k := &lt.kids[i]
				if seen[k.Name] {
					continue
				}
				if f, ok := found[k.Name]; ok && f != k {
					ambiguous[k.Name] = true
					continue
				}
				found[k.Name] = k
			}
			for _, e := range lt.Embeds {
				if et := lt.embedded(e); et != nil {
					next = append(next, et)
				}
			}
		}
		for name, m := range found {
			seen[name] = true
			if !ambiguous[name] {
				out = append(out, m)
			}
		}
		level = next
	}
	return out
}
