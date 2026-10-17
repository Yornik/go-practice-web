package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
)

// staticFiles serves web/static. Every URL a page uses carries
// ?v=<hash of that file's content>, so a deploy changes only the URLs of
// the files that changed, and browsers may keep a file for a year. ES
// modules import each other by plain path; the import map that every page
// carries maps those paths to the versioned URLs. CSS files reference
// fonts with /static/ paths, which are rewritten to versioned URLs before
// the CSS itself is hashed. Monaco's files are fetched by its own loader
// without the query, so they get a day.
type staticFiles struct {
	fsys fs.FS
	dev  bool

	mu    sync.Mutex
	snap  *assetSnapshot
	stamp string // dev only: the names, sizes and times the snapshot was built from
}

// assetSnapshot is the versioned view of the static files at 1 moment.
type assetSnapshot struct {
	versions map[string]string // path below static/ -> short content hash
	bodies   map[string][]byte // CSS files with their /static/ URLs versioned
	// importMap is the whole <script type="importmap"> element; cspHash
	// is the CSP source that allows it ('sha256-...').
	importMap template.HTML
	cspHash   string
}

// vendorDir holds third-party files (Monaco); they are versioned by their
// own release and left out of the hashes, which keeps the start fast.
const vendorDir = "vendor/"

func newStaticFiles(fsys fs.FS, dev bool) (*staticFiles, error) {
	s := &staticFiles{fsys: fsys, dev: dev}
	stamp, err := s.walkStamp()
	if err != nil {
		return nil, err
	}
	snap, err := buildSnapshot(fsys)
	if err != nil {
		return nil, err
	}
	s.snap, s.stamp = snap, stamp
	return s, nil
}

// current returns the snapshot. In dev mode it is built again when a file
// changed, so an edit shows on reload with a new URL.
func (s *staticFiles) current() *assetSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dev {
		return s.snap
	}
	stamp, err := s.walkStamp()
	if err != nil || stamp == s.stamp {
		return s.snap
	}
	if snap, err := buildSnapshot(s.fsys); err == nil {
		s.snap, s.stamp = snap, stamp
	}
	return s.snap
}

// walkStamp describes the files cheaply (no reads), so dev mode can tell
// when it must hash again.
func (s *staticFiles) walkStamp() (string, error) {
	var b strings.Builder
	err := walkFiles(s.fsys, func(p string, d fs.DirEntry) error {
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("web: list static files: %w", err)
	}
	return b.String(), nil
}

// walkFiles calls fn for every file outside vendor/.
func walkFiles(fsys fs.FS, fn func(p string, d fs.DirEntry) error) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p+"/" == vendorDir {
				return fs.SkipDir
			}
			return nil
		}
		return fn(p, d)
	})
}

// cssURL finds url(/static/...) references in CSS.
var cssURL = regexp.MustCompile(`url\(\s*["']?(/static/[^"'?#)\s]+)["']?\s*\)`)

func buildSnapshot(fsys fs.FS) (*assetSnapshot, error) {
	snap := &assetSnapshot{versions: map[string]string{}, bodies: map[string][]byte{}}
	var css []string
	err := walkFiles(fsys, func(p string, _ fs.DirEntry) error {
		if path.Ext(p) == ".css" {
			css = append(css, p)
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		snap.versions[p] = shortHash(b)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("web: hash static files: %w", err)
	}
	// CSS last: its hash must cover the versioned URLs it references.
	for _, p := range css {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("web: read %s: %w", p, err)
		}
		b = cssURL.ReplaceAllFunc(b, func(m []byte) []byte {
			ref := string(cssURL.FindSubmatch(m)[1])
			v, ok := snap.versions[strings.TrimPrefix(ref, "/static/")]
			if !ok {
				return m
			}
			return []byte(`url("` + ref + "?v=" + v + `")`)
		})
		snap.bodies[p] = b
		snap.versions[p] = shortHash(b)
	}

	imports := map[string]string{}
	for p, v := range snap.versions {
		if strings.HasPrefix(p, "js/") && path.Ext(p) == ".js" {
			imports["/static/"+p] = "/static/" + p + "?v=" + v
		}
	}
	// json.Marshal sorts the keys, so the same files give the same hash.
	j, err := json.Marshal(map[string]any{"imports": imports})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(j)
	snap.cspHash = "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	// The JSON is built from file names below web/static, so it holds no
	// markup; json.Marshal also escapes <, > and &.
	snap.importMap = template.HTML(`<script type="importmap">` + string(j) + `</script>`) //nolint:gosec // see above
	return snap, nil
}

func shortHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}

// url returns the versioned URL of a static file. A name that does not
// exist keeps a plain URL, which the 404 then shows.
func (s *staticFiles) url(p string) string {
	if v, ok := s.current().versions[p]; ok {
		return "/static/" + p + "?v=" + v
	}
	return "/static/" + p
}

// contentTypes covers the extensions the standard library does not know
// on every system: a distroless image has no /etc/mime.types.
var contentTypes = map[string]string{
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".ttf":   "font/ttf",
	".woff2": "font/woff2",
	".json":  "application/json",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".txt":   "text/plain; charset=utf-8",
}

func (s *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path
	if name == "" || strings.HasSuffix(name, "/") || !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	f, err := s.fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	rs, ok := f.(io.ReadSeeker)
	if err != nil || st.IsDir() || !ok {
		// No directory listings.
		http.NotFound(w, r)
		return
	}
	snap := s.current()
	v, versioned := snap.versions[name]
	switch {
	case s.dev:
		w.Header().Set("Cache-Control", "no-cache")
	case versioned && r.URL.Query().Get("v") == v:
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasPrefix(name, vendorDir):
		w.Header().Set("Cache-Control", "public, max-age=86400")
	default:
		// An old or missing version: serve the current file, briefly.
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if ct, ok := contentTypes[path.Ext(name)]; ok {
		w.Header().Set("Content-Type", ct)
	}
	if body, ok := snap.bodies[name]; ok {
		rs = bytes.NewReader(body)
	}
	if versioned {
		// Embedded files have no modification time; the ETag lets a
		// browser revalidate a file it got without a version.
		w.Header().Set("ETag", `"`+v+`"`)
	}
	http.ServeContent(w, r, name, st.ModTime(), rs)
}
