package godoc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/godoc"
)

func serve(t *testing.T, h http.Handler, method, target string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func TestHandlerPackage(t *testing.T) {
	ix := newTestIndex(t)
	w := serve(t, godoc.NewHandler(ix), "GET", "/api/docs/pkg?path=strings")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control = %q, want a max-age", cc)
	}
	if w.Header().Get("ETag") != ix.ETag() {
		t.Errorf("ETag = %q, want %q", w.Header().Get("ETag"), ix.ETag())
	}
	got := decode[godoc.PackageDoc](t, w)
	want := godoc.PackageDoc{Path: "strings", Name: "strings", Synopsis: "Package strings manipulates strings.",
		Doc: "Package strings manipulates strings.", URL: "https://pkg.go.dev/strings", GoVersion: "go1.99.1"}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestHandlerMember(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	tests := map[string]godoc.MemberDoc{
		"/api/docs/member?path=strings&name=Cut": {Path: "strings", Package: "strings", Name: "Cut", Kind: godoc.KindFunc,
			Signature: "func Cut(s, sep string) (before, after string, found bool)", Doc: "Cut slices s around sep.",
			URL: "https://pkg.go.dev/strings#Cut"},
		"/api/docs/member?path=strings&name=Builder.WriteString": {Path: "strings", Package: "strings", Name: "WriteString",
			Kind: godoc.KindMethod, Recv: "Builder", Signature: "func (Builder) WriteString()",
			URL: "https://pkg.go.dev/strings#Builder.WriteString"},
		// A promoted method answers with the type that declares it, so the link goes to the right place.
		"/api/docs/member?path=bufio&name=ReadWriter.ReadString": {Path: "bufio", Package: "bufio", Name: "ReadString",
			Kind: godoc.KindMethod, Recv: "Reader", Signature: "func (Reader) ReadString()",
			URL: "https://pkg.go.dev/bufio#Reader.ReadString"},
		"/api/docs/member?path=strings&name=Title": {Path: "strings", Package: "strings", Name: "Title", Kind: godoc.KindFunc,
			Signature: "func Title()", Deprecated: "Use golang.org/x/text/cases instead.", URL: "https://pkg.go.dev/strings#Title"},
	}
	for target, want := range tests {
		w := serve(t, h, "GET", target)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, body %s", target, w.Code, w.Body)
			continue
		}
		if got := decode[godoc.MemberDoc](t, w); got != want {
			t.Errorf("GET %s:\n got %+v\nwant %+v", target, got, want)
		}
	}
}

func TestHandlerComplete(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	w := serve(t, h, "GET", "/api/docs/complete?path=order&prefix=Ti")
	got := decode[godoc.Completions](t, w)
	var gotNames []string
	for _, it := range got.Items {
		gotNames = append(gotNames, it.Name)
	}
	if strings.Join(gotNames, " ") != "Ti TiAlpha Tick TiBeta TIMEOUT tidy" || got.Incomplete || got.Path != "order" || got.Prefix != "Ti" {
		t.Errorf("complete = %+v", got)
	}
	if got.Items[0].URL != "https://pkg.go.dev/order#Ti" {
		t.Errorf("items[0].url = %q", got.Items[0].URL)
	}

	w = serve(t, h, "GET", "/api/docs/complete?path=order&prefix=Ti&limit=2")
	got = decode[godoc.Completions](t, w)
	if len(got.Items) != 2 || got.Items[1].Name != "TiAlpha" || !got.Incomplete {
		t.Errorf("complete with limit=2 = %+v, want 2 items and incomplete", got)
	}

	// An empty prefix lists the package, as after "strings." in the editor.
	w = serve(t, h, "GET", "/api/docs/complete?path=strings")
	if got := decode[godoc.Completions](t, w); len(got.Items) != 7 || got.Items == nil {
		t.Errorf("complete with no prefix = %d items, want 7", len(got.Items))
	}

	// No match is an empty list, not null, so the editor can range over it.
	w = serve(t, h, "GET", "/api/docs/complete?path=strings&prefix=zzz")
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Errorf("complete with no match = %s, want an empty items list", w.Body)
	}
}

func TestHandlerPackages(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	w := serve(t, h, "GET", "/api/docs/packages?prefix=rand")
	got := decode[godoc.PackageList](t, w)
	var paths []string
	for _, it := range got.Items {
		paths = append(paths, it.Path)
	}
	if strings.Join(paths, " ") != "crypto/rand math/rand/v2 math/rand" || got.Items[2].Deprecated == "" {
		t.Errorf("packages = %+v", got)
	}
	w = serve(t, h, "GET", "/api/docs/packages?prefix=&limit=3")
	if got := decode[godoc.PackageList](t, w); len(got.Items) != 3 || !got.Incomplete {
		t.Errorf("packages with limit=3 = %+v", got)
	}
}

func TestHandlerCaching(t *testing.T) {
	ix := newTestIndex(t)
	h := godoc.NewHandler(ix)
	w := serve(t, h, "GET", "/api/docs/member?path=strings&name=Cut", "If-None-Match", ix.ETag())
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Errorf("If-None-Match with the ETag: status %d, body %q; want 304 and no body", w.Code, w.Body)
	}
	w = serve(t, h, "GET", "/api/docs/member?path=strings&name=Cut", "If-None-Match", `"other", W/`+ix.ETag())
	if w.Code != http.StatusNotModified {
		t.Errorf("If-None-Match with a list: status %d, want 304", w.Code)
	}
	w = serve(t, h, "GET", "/api/docs/member?path=strings&name=Cut", "If-None-Match", `"go1.1-0000"`)
	if w.Code != http.StatusOK {
		t.Errorf("If-None-Match with an old ETag: status %d, want 200", w.Code)
	}
	w = serve(t, h, "GET", "/api/docs/member?path=strings&name=Nope", "If-None-Match", ix.ETag())
	if w.Code != http.StatusNotFound || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("a miss: status %d, Cache-Control %q; want 404 and no-store", w.Code, w.Header().Get("Cache-Control"))
	}
}

func TestHandlerMethods(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	w := serve(t, h, "HEAD", "/api/docs/pkg?path=strings")
	if w.Code != http.StatusOK {
		t.Errorf("HEAD: status %d", w.Code)
	}
	w = serve(t, h, "POST", "/api/docs/pkg?path=strings")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: status %d, Allow %q", w.Code, w.Header().Get("Allow"))
	}
	if e := decode[apiError](t, w); e.Code != "method_not_allowed" {
		t.Errorf("POST: error %+v", e)
	}
}

// Error messages may reach the learner, so they explain the usual mistake instead of only saying "not found".
func TestHandlerErrors(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	tests := []struct {
		target string
		status int
		code   string
		says   string
	}{
		{"/api/docs/pkg", 400, "bad_request", "the path parameter is required"},
		{"/api/docs/member?path=strings", 400, "bad_request", "the name parameter is required"},
		{"/api/docs/complete?prefix=x", 400, "bad_request", "the path parameter is required"},
		{"/api/docs/complete?path=strings&limit=0", 400, "bad_request", "limit must be a number from 1 to 500"},
		{"/api/docs/complete?path=strings&limit=501", 400, "bad_request", "limit must be"},
		{"/api/docs/complete?path=strings&limit=ten", 400, "bad_request", "limit must be"},
		{"/api/docs/pkg?path=" + strings.Repeat("a", 300), 400, "bad_request", "too long"},
		{"/api/docs/pkg?path=%ff", 400, "bad_request", "not valid UTF-8"},
		{"/api/docs/pkg?path=http", 404, "not_found", `did you mean "net/http"?`},
		{"/api/docs/pkg?path=rand", 404, "not_found", `"crypto/rand" or "math/rand/v2" or "math/rand"`},
		{"/api/docs/pkg?path=github.com/x/y", 404, "not_found", "Only the standard library is indexed"},
		{"/api/docs/member?path=strings&name=cut", 404, "not_found", "Did you mean Cut? Go names are case-sensitive"},
		{"/api/docs/member?path=strings&name=helper", 404, "not_found", "Only names that start with an upper-case letter are exported"},
		{"/api/docs/member?path=strings&name=Builder.writeString", 404, "not_found", "strings.Builder has no \"writeString\". Did you mean WriteString?"},
		{"/api/docs/member?path=strings&name=Buildr.Len", 404, "not_found", `strings has no "Buildr"`},
		{"/api/docs/member?path=builtin&name=appendd", 404, "not_found", `builtin has no "appendd"`},
		{"/api/docs/complete?path=nope", 404, "not_found", "is not a package of the Go 1.99.1 standard library"},
		{"/api/docs/", 404, "not_found", "unknown docs endpoint"},
		{"/api/docs/search?q=x", 404, "not_found", "unknown docs endpoint"},
	}
	for _, tt := range tests {
		w := serve(t, h, "GET", tt.target)
		if w.Code != tt.status {
			t.Errorf("GET %s: status %d, want %d", tt.target, w.Code, tt.status)
		}
		e := decode[apiError](t, w)
		if e.Code != tt.code || !strings.Contains(e.Error, tt.says) {
			t.Errorf("GET %s: error %+v, want code %s and a message with %q", tt.target, e, tt.code, tt.says)
		}
	}
	if w := serve(t, h, "GET", "/api/docs/member?path=builtin&name=appendd"); strings.Contains(w.Body.String(), "exported") {
		t.Errorf("builtin miss mentions exported names, but builtin names are lower case: %s", w.Body)
	}
}

// The web server may mount the handler with or without http.StripPrefix.
func TestHandlerMounting(t *testing.T) {
	h := godoc.NewHandler(newTestIndex(t))
	mux := http.NewServeMux()
	mux.Handle("/api/docs/", h)
	mux.Handle("/docs/", http.StripPrefix("/docs", h))
	for _, target := range []string{"/api/docs/pkg?path=io", "/docs/pkg?path=io", "/api/docs/pkg/?path=io"} {
		if w := serve(t, mux, "GET", target); w.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, body %s", target, w.Code, w.Body)
		}
	}
}

func TestHandlerServesEmbeddedIndex(t *testing.T) {
	w := serve(t, godoc.Handler(), "GET", "/api/docs/member?path=strings&name=Cut")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	if got := decode[godoc.MemberDoc](t, w); got.URL != "https://pkg.go.dev/strings#Cut" || got.Doc == "" {
		t.Errorf("strings.Cut = %+v", got)
	}
}
