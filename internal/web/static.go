package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// staticFiles serves web/static. Pages link to files with ?v=<digest of
// the files>, so a deploy changes every URL and browsers may cache them
// for a year. Monaco's files are fetched by its own loader without the
// query, so they get a day.
type staticFiles struct {
	fsys    fs.FS
	dev     bool
	version string
}

// vendorDir holds third-party files (Monaco); they are versioned by their
// own release and left out of the digest, which keeps the start fast.
const vendorDir = "vendor/"

func newStaticFiles(fsys fs.FS, dev bool) (*staticFiles, error) {
	s := &staticFiles{fsys: fsys, dev: dev, version: "dev"}
	if dev {
		return s, nil
	}
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p+"/" == vendorDir {
				return fs.SkipDir
			}
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, _ = io.WriteString(h, p+"\x00")
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("web: hash static files: %w", err)
	}
	s.version = hex.EncodeToString(h.Sum(nil))[:12]
	return s, nil
}

// url returns the versioned URL of a static file.
func (s *staticFiles) url(p string) string { return "/static/" + p + "?v=" + s.version }

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
	switch {
	case s.dev:
		w.Header().Set("Cache-Control", "no-cache")
	case r.URL.Query().Get("v") == s.version:
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasPrefix(name, vendorDir):
		w.Header().Set("Cache-Control", "public, max-age=86400")
	default:
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if ct, ok := contentTypes[path.Ext(name)]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, st.ModTime(), rs)
}
