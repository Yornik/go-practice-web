package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticVersions(t *testing.T) {
	fsys := fstest.MapFS{
		"js/api.js":                  {Data: []byte("export const x = 1;\n")},
		"js/ide/main.js":             {Data: []byte(`import { x } from "../api.js";` + "\n")},
		"css/site.css":               {Data: []byte(`@font-face { src: url("/static/fonts/a.woff2") format("woff2"); } a { background: url(/static/img/x.svg); }`)},
		"fonts/a.woff2":              {Data: []byte("font")},
		"img/x.svg":                  {Data: []byte("<svg/>")},
		"vendor/monaco/vs/loader.js": {Data: []byte("monaco")},
	}
	s, err := newStaticFiles(fsys, false)
	if err != nil {
		t.Fatal(err)
	}
	snap := s.current()

	// Every file outside vendor/ has its own version.
	if _, ok := snap.versions["vendor/monaco/vs/loader.js"]; ok {
		t.Error("vendor files are hashed")
	}
	if snap.versions["js/api.js"] == snap.versions["js/ide/main.js"] || len(snap.versions["js/api.js"]) != 12 {
		t.Errorf("versions = %v", snap.versions)
	}
	if got := s.url("js/api.js"); got != "/static/js/api.js?v="+snap.versions["js/api.js"] {
		t.Errorf("url = %s", got)
	}

	// CSS references carry the version of the file they point at.
	css := string(snap.bodies["css/site.css"])
	for _, want := range []string{
		`url("/static/fonts/a.woff2?v=` + snap.versions["fonts/a.woff2"] + `")`,
		`url("/static/img/x.svg?v=` + snap.versions["img/x.svg"] + `")`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("css lacks %s:\n%s", want, css)
		}
	}

	// The import map maps each module path to its versioned URL, and the
	// CSP hash matches the map's text exactly.
	m := regexp.MustCompile(`^<script type="importmap">(.*)</script>$`).FindStringSubmatch(string(snap.importMap))
	if m == nil {
		t.Fatalf("import map = %s", snap.importMap)
	}
	var im struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal([]byte(m[1]), &im); err != nil {
		t.Fatal(err)
	}
	if im.Imports["/static/js/api.js"] != "/static/js/api.js?v="+snap.versions["js/api.js"] || len(im.Imports) != 2 {
		t.Errorf("imports = %v", im.Imports)
	}
	sum := sha256.Sum256([]byte(m[1]))
	if snap.cspHash != "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'" {
		t.Errorf("csp hash %s does not match the map", snap.cspHash)
	}
	if p := pageCSP(snap.cspHash, false); !strings.Contains(p, "script-src 'self' "+snap.cspHash+";") || !strings.Contains(p, "style-src 'self';") {
		t.Errorf("page CSP = %s", p)
	}
	if p := pageCSP(snap.cspHash, true); !strings.Contains(p, "style-src 'self' 'unsafe-inline';") {
		t.Errorf("the IDE's CSP = %s", p)
	}

	// A file with its own version is cached for a year; the rewritten CSS
	// is what the browser gets.
	for _, tc := range []struct{ path, cache string }{
		{"/js/api.js?v=" + snap.versions["js/api.js"], "public, max-age=31536000, immutable"},
		{"/js/api.js?v=old", "public, max-age=300"},
		{"/vendor/monaco/vs/loader.js", "public, max-age=86400"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.URL.Path = strings.SplitN(tc.path, "?", 2)[0][1:]
		s.ServeHTTP(w, r)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("GET %s = %d, Cache-Control %q", tc.path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/css/site.css", nil)
	r.URL.Path = "css/site.css"
	s.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "a.woff2?v=") {
		t.Errorf("served CSS is not rewritten: %s", w.Body.String())
	}
}

// TestDarkTokens checks that the 2 dark blocks of site.css hold the same
// values: 1 applies when the system is dark and the learner chose nothing,
// the other when the learner chose dark.
func TestDarkTokens(t *testing.T) {
	b, err := os.ReadFile("../../web/static/css/site.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	block := func(start string) []string {
		i := strings.Index(css, start)
		if i < 0 {
			t.Fatalf("site.css lacks %q", start)
		}
		body := css[i+len(start):]
		body = body[:strings.Index(body, "}")]
		var decls []string
		for _, d := range strings.Split(body, ";") {
			if d = strings.Join(strings.Fields(d), " "); d != "" {
				decls = append(decls, d)
			}
		}
		slices.Sort(decls)
		return decls
	}
	system := block(`:root:not([data-theme="light"]) {`)
	chosen := block(`:root[data-theme="dark"] {`)
	if len(system) < 20 || !slices.Equal(system, chosen) {
		t.Errorf("the dark blocks differ:\nsystem: %v\nchosen: %v", system, chosen)
	}
}
