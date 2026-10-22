package godoc_test

import (
	"bytes"
	"compress/gzip"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/yornik/go-practice-web/internal/godoc"
)

func fn(name string) godoc.Member {
	return godoc.Member{Name: name, Kind: godoc.KindFunc, Signature: "func " + name + "()"}
}

func typ(name, sig string, embeds ...string) godoc.Member {
	return godoc.Member{Name: name, Kind: godoc.KindType, Signature: "type " + name + " " + sig, Embeds: embeds}
}

func method(recv, name string) godoc.Member {
	return godoc.Member{Name: name, Kind: godoc.KindMethod, Recv: recv, Signature: "func (" + recv + ") " + name + "()"}
}

func field(recv, name, sig string) godoc.Member {
	return godoc.Member{Name: name, Kind: godoc.KindField, Recv: recv, Signature: sig}
}

func deprecated(m godoc.Member, why string) godoc.Member {
	m.Deprecated = why
	return m
}

// newTestIndex builds a small index by hand, in an order Encode must fix, and loads it like the server does.
func newTestIndex(t testing.TB) *godoc.Index {
	t.Helper()
	ix := &godoc.Index{GoVersion: "go1.99.1", GOOS: "linux", GOARCH: "amd64", Packages: []*godoc.Package{
		{Path: "strings", Name: "strings", Synopsis: "Package strings manipulates strings.", Doc: "Package strings manipulates strings.", Members: []godoc.Member{
			method("Builder", "WriteString"),
			{Name: "Cut", Kind: godoc.KindFunc, Signature: "func Cut(s, sep string) (before, after string, found bool)", Doc: "Cut slices s around sep."},
			fn("CutSuffix"), fn("CutPrefix"), fn("Count"), fn("Compare"),
			deprecated(fn("Title"), "Use golang.org/x/text/cases instead."),
			typ("Builder", "struct{ /* unexported fields */ }"),
			method("Builder", "Len"),
		}},
		{Path: "order", Name: "order", Members: []godoc.Member{
			fn("tidy"), fn("TIMEOUT"), fn("Tick"), deprecated(fn("TiBeta"), "Use TiAlpha."), fn("TiAlpha"), fn("Ti"), fn("Other"),
		}},
		{Path: "io", Name: "io", Members: []godoc.Member{
			typ("Reader", "interface{ Read(p []byte) (n int, err error) }"), method("Reader", "Read"),
			typ("Writer", "interface{ Write(p []byte) (n int, err error) }"), method("Writer", "Write"),
			typ("Closer", "interface{ Close() error }"), method("Closer", "Close"),
			typ("ReadWriter", "interface{ ... }", "Reader", "Writer"),
			typ("ReadCloser", "interface{ ... }", "Reader", "Closer"),
			typ("WriteCloser", "interface{ ... }", "Writer", "Closer"),
			// Both embedded interfaces carry Close; that is 1 method, not an ambiguous selector.
			typ("ReadWriteCloser", "interface{ ... }", "ReadCloser", "WriteCloser"),
			{Name: "EOF", Kind: godoc.KindVar, Signature: `var EOF = errors.New("EOF")`},
		}},
		{Path: "bufio", Name: "bufio", Members: []godoc.Member{
			typ("Reader", "struct{ /* unexported fields */ }"), method("Reader", "Size"), method("Reader", "ReadString"),
			typ("Writer", "struct{ /* unexported fields */ }"), method("Writer", "Size"), method("Writer", "Flush"),
			typ("ReadWriter", "struct{ *Reader; *Writer }", "Reader", "Writer"),
			field("ReadWriter", "Reader", "*Reader"), field("ReadWriter", "Writer", "*Writer"),
			// Cycle is legal Go through pointers; lookups must stop.
			typ("Cycle", "struct{ *Cycle; *Reader }", "Cycle", "Reader"),
		}},
		{Path: "builtin", Name: "builtin", Members: []godoc.Member{
			typ("error", "interface{ Error() string }"), method("error", "Error"), fn("append"),
		}},
		{Path: "net", Name: "net", Members: []godoc.Member{
			typ("Error", "interface{ ... }", "builtin.error"), method("Error", "Timeout"),
			typ("Conn", "interface{ ... }", "io.Reader", "missing/pkg.Type"),
		}},
		{Path: "net/http", Name: "http", Members: []godoc.Member{}},
		{Path: "crypto/rand", Name: "rand", Members: []godoc.Member{}},
		{Path: "math/rand/v2", Name: "rand", Members: []godoc.Member{}},
		{Path: "math/rand", Name: "rand", Deprecated: "Use math/rand/v2.", Members: []godoc.Member{}},
		{Path: "io/ioutil", Name: "ioutil", Deprecated: "Use io or os.", Members: []godoc.Member{}},
	}}
	var buf bytes.Buffer
	if err := godoc.Encode(&buf, ix); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	loaded, err := godoc.Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return loaded
}

// selector names a member as "pkg.Recv.Name" for readable failures.
func selector(m *godoc.Member) string {
	if m == nil {
		return "<nil>"
	}
	s := m.Package().Path + "."
	if m.Recv != "" {
		s += m.Recv + "."
	}
	return s + m.Name
}

func TestPackage(t *testing.T) {
	ix := newTestIndex(t)
	p := ix.Package("strings")
	if p == nil || p.Name != "strings" || p.Synopsis != "Package strings manipulates strings." {
		t.Fatalf("Package(strings) = %+v", p)
	}
	if got, want := p.URL(), "https://pkg.go.dev/strings"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if ix.Package("http") != nil {
		t.Error(`Package("http") != nil; lookups use the import path`)
	}
	if !strings.HasPrefix(ix.ETag(), `"go1.99.1-`) {
		t.Errorf("ETag = %s, want the Go version and a hash", ix.ETag())
	}
}

func TestMember(t *testing.T) {
	ix := newTestIndex(t)
	tests := []struct {
		pkg, name string
		want      string // selector, or "<nil>"
		url       string
	}{
		{"strings", "Cut", "strings.Cut", "https://pkg.go.dev/strings#Cut"},
		{"strings", "Builder", "strings.Builder", "https://pkg.go.dev/strings#Builder"},
		{"strings", "Builder.WriteString", "strings.Builder.WriteString", "https://pkg.go.dev/strings#Builder.WriteString"},
		{"builtin", "append", "builtin.append", "https://pkg.go.dev/builtin#append"},
		{"strings", "cut", "<nil>", ""},             // names are case-sensitive
		{"strings", "WriteString", "<nil>", ""},     // a method is not package-level
		{"strings", "Cut.Len", "<nil>", ""},         // Cut is not a type
		{"strings", "Builder.Missing", "<nil>", ""}, // no such method
		{"nope", "Cut", "<nil>", ""},
	}
	for _, tt := range tests {
		m := ix.Member(tt.pkg, tt.name)
		if got := selector(m); got != tt.want {
			t.Errorf("Member(%q, %q) = %s, want %s", tt.pkg, tt.name, got, tt.want)
			continue
		}
		if m != nil && m.URL() != tt.url {
			t.Errorf("Member(%q, %q).URL() = %q, want %q", tt.pkg, tt.name, m.URL(), tt.url)
		}
	}
	if m := ix.Member("strings", "Cut"); m.Signature != "func Cut(s, sep string) (before, after string, found bool)" || m.Doc != "Cut slices s around sep." {
		t.Errorf("strings.Cut = %+v", m)
	}
}

// Method follows embedded types like the compiler: shallowest wins, 2 at the same depth are ambiguous.
func TestMethod(t *testing.T) {
	ix := newTestIndex(t)
	tests := []struct {
		pkg, typ, name string
		want           string
	}{
		{"strings", "Builder", "Len", "strings.Builder.Len"},
		{"io", "ReadWriter", "Read", "io.Reader.Read"},
		{"io", "ReadWriter", "Write", "io.Writer.Write"},
		{"io", "ReadWriteCloser", "Close", "io.Closer.Close"},
		{"io", "ReadWriteCloser", "Read", "io.Reader.Read"},
		{"bufio", "ReadWriter", "ReadString", "bufio.Reader.ReadString"},
		{"bufio", "ReadWriter", "Flush", "bufio.Writer.Flush"},
		{"bufio", "ReadWriter", "Reader", "bufio.ReadWriter.Reader"}, // the field, at depth 0
		{"bufio", "ReadWriter", "Size", "<nil>"},                     // Reader.Size and Writer.Size: ambiguous
		{"bufio", "Cycle", "ReadString", "bufio.Reader.ReadString"},
		{"bufio", "Cycle", "Missing", "<nil>"},
		{"net", "Error", "Error", "builtin.error.Error"},
		{"net", "Error", "Timeout", "net.Error.Timeout"},
		{"net", "Conn", "Read", "io.Reader.Read"},
		{"net", "Conn", "Write", "<nil>"},
		{"io", "EOF", "Error", "<nil>"}, // a var, not a type
		{"io", "Nope", "Read", "<nil>"},
		{"nope", "Reader", "Read", "<nil>"},
	}
	for _, tt := range tests {
		if got := selector(ix.Method(tt.pkg, tt.typ, tt.name)); got != tt.want {
			t.Errorf("Method(%q, %q, %q) = %s, want %s", tt.pkg, tt.typ, tt.name, got, tt.want)
		}
	}
}

func names(ms []*godoc.Member) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

func TestComplete(t *testing.T) {
	ix := newTestIndex(t)
	tests := []struct {
		pkg, prefix string
		want        string
	}{
		{"strings", "Cu", "Cut CutPrefix CutSuffix"},
		{"strings", "Cut", "Cut CutPrefix CutSuffix"},
		{"strings", "CutS", "CutSuffix"},
		{"strings", "cu", "Cut CutPrefix CutSuffix"}, // case-insensitive, so a learner who types "cu" finds Cut
		{"strings", "Co", "Compare Count"},
		{"strings", "", "Builder Compare Count Cut CutPrefix CutSuffix Title"}, // deprecated last
		{"strings", "Builder.", "Len WriteString"},
		{"strings", "Builder.w", "WriteString"},
		{"strings", "x", ""},
		// Exact match, then case-sensitive prefixes (deprecated last), then case-insensitive ones.
		{"order", "Ti", "Ti TiAlpha Tick TiBeta TIMEOUT tidy"},
		{"bufio", "ReadWriter.", "Flush ReadString Reader Writer"}, // Size is ambiguous, so it is left out
		{"bufio", "ReadWriter.Re", "ReadString Reader"},
		{"io", "ReadWriteCloser.", "Close Read Write"},
		{"net", "Error.", "Error Timeout"},
		{"strings", "Nope.", ""},
		{"nope", "", ""},
	}
	for _, tt := range tests {
		if got := strings.Join(names(ix.Complete(tt.pkg, tt.prefix)), " "); got != tt.want {
			t.Errorf("Complete(%q, %q) = %q, want %q", tt.pkg, tt.prefix, got, tt.want)
		}
	}
}

func TestCompletePackages(t *testing.T) {
	ix := newTestIndex(t)
	tests := map[string]string{
		"rand":    "crypto/rand math/rand/v2 math/rand", // exact names; the deprecated one last
		"math/":   "math/rand/v2 math/rand",
		"io":      "io io/ioutil",
		"htt":     "net/http",
		"net":     "net net/http",
		"zzz":     "",
		"strings": "strings",
	}
	for prefix, want := range tests {
		var got []string
		for _, p := range ix.CompletePackages(prefix) {
			got = append(got, p.Path)
		}
		if g := strings.Join(got, " "); g != want {
			t.Errorf("CompletePackages(%q) = %q, want %q", prefix, g, want)
		}
	}
}

// Hover runs on every mouse move; lookups must not allocate.
func TestLookupsDoNotAllocate(t *testing.T) {
	ix := newTestIndex(t)
	allocs := testing.AllocsPerRun(100, func() {
		_ = ix.Package("strings")
		_ = ix.Member("strings", "Cut")
		_ = ix.Member("strings", "Builder.WriteString")
		_ = ix.Method("io", "ReadWriteCloser", "Close")
		_ = ix.Method("net", "Error", "Error")
		_ = ix.Method("bufio", "ReadWriter", "Size")
	})
	if allocs != 0 {
		t.Errorf("lookups allocate %.0f times per run, want 0", allocs)
	}
}

func encode(t *testing.T, json string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(json)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestLoadRejectsBadIndexes(t *testing.T) {
	tests := map[string]string{
		"wrong format":      `{"format": 99, "packages": []}`,
		"not JSON":          `{"format": 1,`,
		"package no path":   `{"format": 1, "packages": [{"name": "x", "members": []}]}`,
		"package twice":     `{"format": 1, "packages": [{"path": "x", "members": []}, {"path": "x", "members": []}]}`,
		"member twice":      `{"format": 1, "packages": [{"path": "x", "members": [{"name": "A", "kind": "func"}, {"name": "A", "kind": "var"}]}]}`,
		"unknown kind":      `{"format": 1, "packages": [{"path": "x", "members": [{"name": "A", "kind": "macro"}]}]}`,
		"method of nothing": `{"format": 1, "packages": [{"path": "x", "members": [{"name": "M", "kind": "method", "recv": "T"}]}]}`,
	}
	for name, json := range tests {
		if _, err := godoc.Load(encode(t, json)); err == nil {
			t.Errorf("%s: Load succeeded, want an error", name)
		}
	}
	if _, err := godoc.Load(strings.NewReader("not gzip")); err == nil {
		t.Error("Load of plain text succeeded, want an error")
	}
	if _, err := godoc.LoadFile("testdata/does-not-exist.json.gz"); err == nil {
		t.Error("LoadFile of a missing file succeeded, want an error")
	}
}

// The embedded index is generated from a real GOROOT; these checks catch a broken generator or a stale file.
func TestDefault(t *testing.T) {
	ix, err := godoc.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if again, _ := godoc.Default(); again != ix {
		t.Error("Default loads the index again on the 2nd call")
	}
	if !strings.HasPrefix(ix.GoVersion, "go1.") || ix.GOOS != "linux" || ix.GOARCH != "amd64" {
		t.Errorf("index is for %s %s/%s, want a Go 1 release for linux/amd64", ix.GoVersion, ix.GOOS, ix.GOARCH)
	}
	if len(ix.Packages) < 150 {
		t.Errorf("index has %d packages, want the whole standard library", len(ix.Packages))
	}
	for _, path := range []string{"internal/abi", "cmd/go", "vendor/golang.org/x/net/dns/dnsmessage", "runtime/cgo", "syscall/js"} {
		if ix.Package(path) != nil {
			t.Errorf("index has %s, which learners cannot import", path)
		}
	}
	tests := []struct {
		pkg, name string
		check     func(m *godoc.Member) bool
	}{
		{"strings", "Cut", func(m *godoc.Member) bool {
			return m.Signature == "func Cut(s, sep string) (before, after string, found bool)" && strings.HasPrefix(m.Doc, "Cut slices s")
		}},
		{"net/http", "Server.Shutdown", func(m *godoc.Member) bool {
			return m.Signature == "func (s *Server) Shutdown(ctx context.Context) error"
		}},
		{"net/http", "Server.Addr", func(m *godoc.Member) bool { return m.Kind == godoc.KindField && m.Signature == "Addr string" }},
		{"context", "Context", func(m *godoc.Member) bool { return strings.Contains(m.Signature, "Done() <-chan struct{}") }},
		{"builtin", "append", func(m *godoc.Member) bool { return strings.HasPrefix(m.Signature, "func append(") }},
		{"builtin", "len", func(m *godoc.Member) bool { return strings.Contains(m.Doc, "Channel:") }},
		{"io/ioutil", "ReadAll", func(m *godoc.Member) bool { return strings.Contains(m.Deprecated, "io.ReadAll") }},
		{"sync", "WaitGroup", func(m *godoc.Member) bool { return strings.Contains(m.Signature, "unexported fields") }},
		{"time", "Second", func(m *godoc.Member) bool { return m.Kind == godoc.KindConst }},
		{"os/signal", "NotifyContext", func(m *godoc.Member) bool { return strings.HasPrefix(m.Signature, "func NotifyContext(") }},
		{"syscall", "SIGTERM", func(m *godoc.Member) bool { return m.Kind == godoc.KindConst }},
	}
	for _, tt := range tests {
		m := ix.Member(tt.pkg, tt.name)
		if m == nil {
			t.Errorf("Member(%q, %q) = nil", tt.pkg, tt.name)
			continue
		}
		if !tt.check(m) {
			t.Errorf("Member(%q, %q) = %+v", tt.pkg, tt.name, m)
		}
	}
	if m := ix.Method("io", "ReadWriter", "Read"); selector(m) != "io.Reader.Read" {
		t.Errorf("io.ReadWriter.Read resolves to %s, want io.Reader.Read", selector(m))
	}
	if m := ix.Method("net", "Error", "Error"); selector(m) != "builtin.error.Error" {
		t.Errorf("net.Error.Error resolves to %s, want builtin.error.Error", selector(m))
	}
	if m := ix.Method("bufio", "ReadWriter", "Size"); m != nil {
		t.Errorf("bufio.ReadWriter.Size resolves to %s; the compiler reports it as ambiguous", selector(m))
	}
	if p := ix.Package("io/ioutil"); p == nil || p.Deprecated == "" {
		t.Error("io/ioutil is not marked deprecated")
	}
}

// Renovate moves the -go flag of the go:generate line with the runner image,
// but it cannot regenerate the index. Without this check a Go pull request
// passes CI and ships the docs of the old release to the editor.
func TestIndexRelease(t *testing.T) {
	src, err := os.ReadFile("godoc.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^//go:generate go run \.\./\.\./cmd/gendocs -go (go1\.\d+) `).FindSubmatch(src)
	if m == nil {
		t.Fatal("godoc.go has no go:generate line with -go go1.N")
	}
	ix, err := godoc.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	want := string(m[1])
	if got := ix.GoVersion; got != want && !strings.HasPrefix(got, want+".") {
		t.Errorf("the index is from %s, and the go:generate line wants %s: regenerate it with GOTOOLCHAIN=%s.N go generate ./internal/godoc", got, want, want)
	}
}

func BenchmarkMember(b *testing.B) {
	ix, err := godoc.Default()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ix.Member("net/http", "Server.Shutdown")
	}
}

func BenchmarkMethodPromoted(b *testing.B) {
	ix, err := godoc.Default()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ix.Method("io", "ReadWriteCloser", "Close")
	}
}

func BenchmarkComplete(b *testing.B) {
	ix, err := godoc.Default()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ix.Complete("net/http", "Stat")
	}
}

func BenchmarkLoad(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := godoc.LoadFile("data/stdlib.json.gz"); err != nil {
			b.Fatal(err)
		}
	}
}
