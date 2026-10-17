package godoc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	defaultLimit = 100
	maxLimit     = 500
	maxParamLen  = 256
	// The docs only change with a deploy, and the ETag catches that; a day keeps hover and completion off the
	// network while a learner types.
	cacheControl = "public, max-age=86400"
)

// Handler serves the embedded index (see Default). It loads the index on the first request.
//
// Mount it at /api/docs/ with or without http.StripPrefix: it routes on the last path element.
//
//	GET pkg?path=strings                     the package
//	GET member?path=strings&name=Cut         a package-level member, or name=Builder.WriteString for a method or field
//	GET complete?path=strings&prefix=Cu      members for completion, best first; limit=N (default 100, max 500)
//	GET packages?prefix=net/                 packages for import completion; limit=N
//
// Responses are JSON with an ETag and Cache-Control. Errors are {"error": "...", "code": "..."}.
func Handler() http.Handler {
	return &handler{load: Default}
}

// NewHandler serves ix like Handler serves the embedded index.
func NewHandler(ix *Index) http.Handler {
	return &handler{load: func() (*Index, error) { return ix, nil }}
}

type handler struct {
	load    func() (*Index, error)
	logOnce sync.Once
}

// PackageDoc is the JSON shape of a package.
type PackageDoc struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Synopsis   string `json:"synopsis,omitempty"`
	Doc        string `json:"doc,omitempty"`
	Deprecated string `json:"deprecated,omitempty"`
	URL        string `json:"url"`
	GoVersion  string `json:"goVersion,omitempty"`
}

// MemberDoc is the JSON shape of a member.
type MemberDoc struct {
	Path       string `json:"path"`
	Package    string `json:"package"`
	Name       string `json:"name"`
	Kind       Kind   `json:"kind"`
	Recv       string `json:"recv,omitempty"`
	Signature  string `json:"signature"`
	Doc        string `json:"doc,omitempty"`
	Deprecated string `json:"deprecated,omitempty"`
	URL        string `json:"url"`
}

// Completions is the JSON shape of a completion response. Incomplete is true when the limit cut the list, so the
// editor asks again as the learner types more.
type Completions struct {
	Path       string      `json:"path"`
	Prefix     string      `json:"prefix"`
	Items      []MemberDoc `json:"items"`
	Incomplete bool        `json:"incomplete"`
}

// PackageList is the JSON shape of a packages response.
type PackageList struct {
	Prefix     string       `json:"prefix"`
	Items      []PackageDoc `json:"items"`
	Incomplete bool         `json:"incomplete"`
}

func packageDoc(p *Package) PackageDoc {
	return PackageDoc{
		Path: p.Path, Name: p.Name, Synopsis: p.Synopsis, Doc: p.Doc, Deprecated: p.Deprecated, URL: p.URL(),
	}
}

func memberDoc(m *Member) MemberDoc {
	return MemberDoc{
		Path: m.pkg.Path, Package: m.pkg.Name, Name: m.Name, Kind: m.Kind, Recv: m.Recv, Signature: m.Signature,
		Doc: m.Doc, Deprecated: m.Deprecated, URL: m.URL(),
	}
}

// apiError is a client mistake; its message is written for the learner, because the editor may show it.
type apiError struct {
	status int
	code   string
	msg    string
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, &apiError{http.StatusMethodNotAllowed, "method_not_allowed", "use GET"})
		return
	}
	ix, err := h.load()
	if err != nil {
		// The load result is cached, so the cause is the same on every request; 1 log line is enough.
		h.logOnce.Do(func() { slog.Error("standard library docs unavailable", "err", err) })
		writeError(w, &apiError{http.StatusServiceUnavailable, "docs_unavailable", "the standard library docs are not available on this server"})
		return
	}
	q := r.URL.Query()
	var body any
	var aerr *apiError
	switch path.Base(r.URL.Path) {
	case "pkg":
		body, aerr = servePackage(ix, q.Get("path"))
	case "member":
		body, aerr = serveMember(ix, q.Get("path"), q.Get("name"))
	case "complete":
		body, aerr = serveComplete(ix, q.Get("path"), q.Get("prefix"), q.Get("limit"))
	case "packages":
		body, aerr = servePackages(ix, q.Get("prefix"), q.Get("limit"))
	default:
		aerr = &apiError{http.StatusNotFound, "not_found", "unknown docs endpoint; use pkg, member, complete or packages"}
	}
	if aerr != nil {
		writeError(w, aerr)
		return
	}
	hdr := w.Header()
	hdr.Set("Cache-Control", cacheControl)
	if etag := ix.ETag(); etag != "" {
		hdr.Set("ETag", etag)
		if etagMatch(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	writeJSON(w, http.StatusOK, body)
}

func servePackage(ix *Index, pkgPath string) (any, *apiError) {
	if aerr := checkParam("path", pkgPath, true); aerr != nil {
		return nil, aerr
	}
	p := ix.Package(pkgPath)
	if p == nil {
		return nil, unknownPackage(ix, pkgPath)
	}
	d := packageDoc(p)
	d.GoVersion = ix.GoVersion
	return d, nil
}

func serveMember(ix *Index, pkgPath, name string) (any, *apiError) {
	if aerr := checkParam("path", pkgPath, true); aerr != nil {
		return nil, aerr
	}
	if aerr := checkParam("name", name, true); aerr != nil {
		return nil, aerr
	}
	p := ix.Package(pkgPath)
	if p == nil {
		return nil, unknownPackage(ix, pkgPath)
	}
	m := p.Member(name)
	if m == nil {
		return nil, unknownMember(p, name)
	}
	return memberDoc(m), nil
}

func serveComplete(ix *Index, pkgPath, prefix, limitParam string) (any, *apiError) {
	if aerr := checkParam("path", pkgPath, true); aerr != nil {
		return nil, aerr
	}
	if aerr := checkParam("prefix", prefix, false); aerr != nil {
		return nil, aerr
	}
	limit, aerr := parseLimit(limitParam)
	if aerr != nil {
		return nil, aerr
	}
	p := ix.Package(pkgPath)
	if p == nil {
		return nil, unknownPackage(ix, pkgPath)
	}
	ms := p.Complete(prefix)
	out := Completions{Path: pkgPath, Prefix: prefix, Items: make([]MemberDoc, 0, min(len(ms), limit))}
	if len(ms) > limit {
		ms, out.Incomplete = ms[:limit], true
	}
	for _, m := range ms {
		out.Items = append(out.Items, memberDoc(m))
	}
	return out, nil
}

func servePackages(ix *Index, prefix, limitParam string) (any, *apiError) {
	if aerr := checkParam("prefix", prefix, false); aerr != nil {
		return nil, aerr
	}
	limit, aerr := parseLimit(limitParam)
	if aerr != nil {
		return nil, aerr
	}
	ps := ix.CompletePackages(prefix)
	out := PackageList{Prefix: prefix, Items: make([]PackageDoc, 0, min(len(ps), limit))}
	if len(ps) > limit {
		ps, out.Incomplete = ps[:limit], true
	}
	for _, p := range ps {
		out.Items = append(out.Items, packageDoc(p))
	}
	return out, nil
}

func checkParam(name, value string, required bool) *apiError {
	switch {
	case required && value == "":
		return &apiError{http.StatusBadRequest, "bad_request", "the " + name + " parameter is required"}
	case len(value) > maxParamLen:
		return &apiError{http.StatusBadRequest, "bad_request", "the " + name + " parameter is too long"}
	case !utf8.ValidString(value):
		return &apiError{http.StatusBadRequest, "bad_request", "the " + name + " parameter is not valid UTF-8"}
	}
	return nil
}

func parseLimit(s string) (int, *apiError) {
	if s == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maxLimit {
		return 0, &apiError{http.StatusBadRequest, "bad_request", fmt.Sprintf("limit must be a number from 1 to %d", maxLimit)}
	}
	return n, nil
}

// unknownPackage explains the usual cause: a package name where the import path belongs.
func unknownPackage(ix *Index, pkgPath string) *apiError {
	msg := fmt.Sprintf("%q is not a package of the Go %s standard library", pkgPath, strings.TrimPrefix(ix.GoVersion, "go"))
	var paths []string
	for _, p := range ix.CompletePackages(pkgPath) {
		if p.Name == pkgPath && p.Path != pkgPath {
			paths = append(paths, strconv.Quote(p.Path))
		}
	}
	if len(paths) > 0 {
		msg += ". An import path names the whole path, not only the package name: did you mean " + strings.Join(paths, " or ") + "?"
	} else {
		msg += ". Only the standard library is indexed."
	}
	return &apiError{http.StatusNotFound, "not_found", msg}
}

// unknownMember explains the 2 mistakes behind most misses: wrong case, and names that are not exported.
func unknownMember(p *Package, name string) *apiError {
	owner, sel := p.Path, name
	var candidates []*Member
	if typ, s, ok := strings.Cut(name, "."); ok {
		if p.typ(typ) == nil {
			return unknownMember(p, typ)
		}
		owner, sel = p.Path+"."+typ, s
		candidates = p.typ(typ).methodSet()
	} else {
		for i := range p.Members[:p.nTop] {
			candidates = append(candidates, &p.Members[i])
		}
	}
	msg := fmt.Sprintf("%s has no %q", owner, sel)
	for _, m := range candidates {
		if m.Name != sel && strings.EqualFold(m.Name, sel) {
			msg += fmt.Sprintf(". Did you mean %s? Go names are case-sensitive", m.Name)
			return &apiError{http.StatusNotFound, "not_found", msg}
		}
	}
	if r, _ := utf8.DecodeRuneInString(sel); unicode.IsLower(r) && p.Path != "builtin" {
		msg += ". Only names that start with an upper-case letter are exported, so code outside the package cannot use the others"
	}
	return &apiError{http.StatusNotFound, "not_found", msg}
}

func etagMatch(header, etag string) bool {
	for t := range strings.SplitSeq(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || strings.TrimPrefix(t, "W/") == etag {
			return true
		}
	}
	return false
}

func writeError(w http.ResponseWriter, e *apiError) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, e.status, map[string]string{"error": e.msg, "code": e.code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	hdr := w.Header()
	hdr.Set("Content-Type", "application/json; charset=utf-8")
	hdr.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// The status line is out; an encoding error here means the client went away.
	_ = json.NewEncoder(w).Encode(v)
}
