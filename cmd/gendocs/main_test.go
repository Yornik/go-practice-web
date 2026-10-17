package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yornik/go-practice-web/internal/godoc"
)

// fakeGOROOT is a tiny Go installation: enough packages to cover every kind of member, the skip rules and the
// build constraints, without depending on the Go release that runs the tests.
var fakeGOROOT = map[string]string{
	"VERSION": "go1.99.3\ntime 2026-01-01T00:00:00Z\n",
	"src/strs/strs.go": `// Package strs is a small stand-in for strings.
//
// It has a second paragraph.
package strs

import "errors"

// ErrEmpty is returned for empty input.
var ErrEmpty = errors.New("strs: empty input")

// Cut slices s around the first instance of sep. See [Builder] and [errors.New].
//
// A second paragraph that the index leaves out.
func Cut(s, sep string) (before, after string, found bool) { return }

// Join joins the parts.
func Join(
	parts []string,
	sep string,
) string {
	return ""
}

// Map applies f to every element.
func Map[T any](s []T, f func(T) T) []T { return nil }

// Title upper-cases words.
//
// Deprecated: Use Cut.
func Title(s string) string { return s }

// Compare returns
//
//	-1 if a < b,
//	+1 if a > b.
func Compare(a, b string) int { return 0 }

func helper() {}

// A Builder builds strings.
type Builder struct{ buf []byte }

// NewBuilder returns an empty Builder.
func NewBuilder() *Builder { return nil }

// WriteString appends s.
func (b *Builder) WriteString(s string) (int, error) { return 0, nil }

func (b *Builder) grow() {}

// Point is a position.
type Point struct{ X, Y int }

// Reader reads.
type Reader interface {
	// Read reads into p.
	Read(p []byte) (n int, err error)
}

// Closer closes.
type Closer interface{ Close() error }

// ReadCloser groups Read and Close.
type ReadCloser interface {
	Reader
	Closer
}

// Pair has a name and a builder.
type Pair struct {
	*Builder
	// Name names the pair.
	Name   string
	secret int
}

// Stats has a nested struct.
type Stats struct {
	// BySize counts per size.
	BySize [2]struct {
		// Size is the size.
		Size uint32
	}
}

// Kind is a kind.
type Kind int

// The kinds.
const (
	KindA Kind = iota // the first kind
	KindB
)

// Untyped constants.
const (
	Small = 1
	Large = 1 << 40
)
`,
	"src/strs/strs_linux.go":   "package strs\n\n// LinuxOnly exists on Linux.\nfunc LinuxOnly() {}\n",
	"src/strs/strs_windows.go": "package strs\n\n// WindowsOnly exists on Windows.\nfunc WindowsOnly() {}\n",
	"src/strs/new.go":          "//go:build go1.99\n\npackage strs\n\n// New exists from Go 1.99.\nfunc New() {}\n",
	"src/strs/future.go":       "//go:build go1.100\n\npackage strs\n\n// Future does not exist yet.\nfunc Future() {}\n",
	"src/strs/strs_test.go":    "package strs\n\n// TestOnly is in a test file.\nfunc TestOnly() {}\n",

	"src/strs/internal/hidden/hidden.go": "package hidden\n\nfunc Hidden() {}\n",
	"src/internal/abi/abi.go":            "package abi\n\nfunc ABI() {}\n",
	"src/vendor/golang.org/x/net/net.go": "package net\n\nfunc Vendored() {}\n",
	"src/cmd/vet/main.go":                "package main\n\nfunc main() {}\n",
	"src/runtime/cgo/cgo.go":             "package cgo\n\nfunc Cgo() {}\n",
	"src/testdata/x.go":                  "package x\n\nfunc X() {}\n",
	"src/tool/main.go":                   "package main\n\nfunc main() {}\n",
	"src/ignored/gen.go":                 "//go:build ignore\n\npackage main\n",

	"src/builtin/builtin.go": `// Package builtin documents the predeclared identifiers.
package builtin

// The error built-in interface type is the conventional interface for representing an error condition.
type error interface {
	Error() string
}

// Type is a stand-in for any Go type.
type Type int

// The append built-in function appends elements to the end of a slice.
func append(slice []Type, elems ...Type) []Type
`,
	"src/iox/iox.go": `// Package iox has I/O interfaces.
package iox

// Reader reads.
type Reader interface{ Read(p []byte) (n int, err error) }
`,
	"src/files/files.go": `// Package files embeds types of other packages.
package files

import "iox"

// File is an open file.
type File interface {
	iox.Reader
	Stat() error
}

// Error is a file error.
type Error interface {
	error
	Path() string
}
`,
}

func writeFakeGOROOT(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range fakeGOROOT {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// generateLoaded runs the generator and loads the result the way the web server does.
func generateLoaded(t *testing.T) (*godoc.Index, []byte) {
	t.Helper()
	ix, err := generate(config{goroot: writeFakeGOROOT(t), goos: "linux", goarch: "amd64"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var buf bytes.Buffer
	if err := godoc.Encode(&buf, ix); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	encoded := slices.Clone(buf.Bytes())
	loaded, err := godoc.Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return loaded, encoded
}

func TestGeneratePackages(t *testing.T) {
	ix, _ := generateLoaded(t)
	if ix.GoVersion != "go1.99.3" || ix.GOOS != "linux" || ix.GOARCH != "amd64" {
		t.Errorf("version = %s %s/%s, want go1.99.3 linux/amd64", ix.GoVersion, ix.GOOS, ix.GOARCH)
	}
	var paths []string
	for _, p := range ix.Packages {
		paths = append(paths, p.Path)
	}
	// internal, vendor, cmd, testdata, runtime/cgo, package main and ignored files are all left out.
	want := []string{"builtin", "files", "iox", "strs"}
	if !slices.Equal(paths, want) {
		t.Errorf("packages = %q, want %q", paths, want)
	}
	p := ix.Package("strs")
	if p.Name != "strs" || p.Synopsis != "Package strs is a small stand-in for strings." || p.Doc != p.Synopsis {
		t.Errorf("strs = name %q, synopsis %q, doc %q", p.Name, p.Synopsis, p.Doc)
	}
}

func TestGenerateMembers(t *testing.T) {
	ix, _ := generateLoaded(t)
	tests := []struct {
		pkg, name  string
		kind       godoc.Kind
		recv       string
		signature  string
		doc        string
		deprecated string
	}{
		{"strs", "Cut", godoc.KindFunc, "", "func Cut(s, sep string) (before, after string, found bool)",
			"Cut slices s around the first instance of sep. See Builder and errors.New.", ""},
		{"strs", "Join", godoc.KindFunc, "", "func Join(parts []string, sep string) string", "Join joins the parts.", ""},
		{"strs", "Map", godoc.KindFunc, "", "func Map[T any](s []T, f func(T) T) []T", "Map applies f to every element.", ""},
		{"strs", "Title", godoc.KindFunc, "", "func Title(s string) string", "Title upper-cases words.", "Use Cut."},
		{"strs", "Compare", godoc.KindFunc, "", "func Compare(a, b string) int", "Compare returns: -1 if a < b, +1 if a > b.", ""},
		{"strs", "NewBuilder", godoc.KindFunc, "", "func NewBuilder() *Builder", "NewBuilder returns an empty Builder.", ""},
		{"strs", "ErrEmpty", godoc.KindVar, "", `var ErrEmpty = errors.New("strs: empty input")`, "ErrEmpty is returned for empty input.", ""},
		{"strs", "KindA", godoc.KindConst, "", "const KindA Kind = iota", "the first kind. The kinds.", ""},
		{"strs", "KindB", godoc.KindConst, "", "const KindB Kind", "The kinds.", ""},
		{"strs", "Large", godoc.KindConst, "", "const Large = 1 << 40", "Untyped constants.", ""},
		{"strs", "Builder", godoc.KindType, "", "type Builder struct{ /* unexported fields */ }", "A Builder builds strings.", ""},
		{"strs", "Builder.WriteString", godoc.KindMethod, "Builder", "func (b *Builder) WriteString(s string) (int, error)", "WriteString appends s.", ""},
		{"strs", "Point", godoc.KindType, "", "type Point struct{ X, Y int }", "Point is a position.", ""},
		{"strs", "Point.Y", godoc.KindField, "Point", "Y int", "", ""},
		{"strs", "Reader", godoc.KindType, "", "type Reader interface{ Read(p []byte) (n int, err error) }", "Reader reads.", ""},
		{"strs", "Reader.Read", godoc.KindMethod, "Reader", "Read(p []byte) (n int, err error)", "Read reads into p.", ""},
		{"strs", "ReadCloser", godoc.KindType, "", "type ReadCloser interface {\n\tReader\n\tCloser\n}", "ReadCloser groups Read and Close.", ""},
		{"strs", "Pair", godoc.KindType, "", "type Pair struct{ *Builder; Name string; /* unexported fields */ }", "Pair has a name and a builder.", ""},
		{"strs", "Pair.Builder", godoc.KindField, "Pair", "*Builder", "", ""},
		{"strs", "Pair.Name", godoc.KindField, "Pair", "Name string", "Name names the pair.", ""},
		{"strs", "Stats.BySize", godoc.KindField, "Stats", "BySize [2]struct { Size uint32 }", "BySize counts per size.", ""},
		{"strs", "LinuxOnly", godoc.KindFunc, "", "func LinuxOnly()", "LinuxOnly exists on Linux.", ""},
		{"strs", "New", godoc.KindFunc, "", "func New()", "New exists from Go 1.99.", ""},
		{"builtin", "append", godoc.KindFunc, "", "func append(slice []Type, elems ...Type) []Type",
			"The append built-in function appends elements to the end of a slice.", ""},
		{"builtin", "error", godoc.KindType, "", "type error interface{ Error() string }",
			"The error built-in interface type is the conventional interface for representing an error condition.", ""},
		{"builtin", "error.Error", godoc.KindMethod, "error", "Error() string", "", ""},
		{"files", "File", godoc.KindType, "", "type File interface {\n\tiox.Reader\n\tStat() error\n}", "File is an open file.", ""},
	}
	for _, tt := range tests {
		t.Run(tt.pkg+"."+tt.name, func(t *testing.T) {
			m := ix.Member(tt.pkg, tt.name)
			if m == nil {
				t.Fatalf("Member(%q, %q) = nil", tt.pkg, tt.name)
			}
			if m.Kind != tt.kind || m.Recv != tt.recv {
				t.Errorf("kind, recv = %s, %q; want %s, %q", m.Kind, m.Recv, tt.kind, tt.recv)
			}
			if m.Signature != tt.signature {
				t.Errorf("signature:\n got %q\nwant %q", m.Signature, tt.signature)
			}
			if m.Doc != tt.doc {
				t.Errorf("doc:\n got %q\nwant %q", m.Doc, tt.doc)
			}
			if m.Deprecated != tt.deprecated {
				t.Errorf("deprecated = %q, want %q", m.Deprecated, tt.deprecated)
			}
		})
	}
}

func TestGenerateLeavesOut(t *testing.T) {
	ix, _ := generateLoaded(t)
	for _, name := range []string{
		"helper",            // unexported
		"Builder.grow",      // unexported method
		"Pair.secret",       // unexported field
		"WindowsOnly",       // another GOOS
		"Future",            // a newer release tag
		"TestOnly",          // a _test.go file
		"Builder.buf",       // unexported field of an opaque type
		"ReadCloser.Reader", // an embedded interface is not a method
		"Stats.BySize.Size", // nested fields are part of the signature only
		"KindA.String",      // no such method
		"Point.X.Y",         // nonsense
	} {
		if m := ix.Member("strs", name); m != nil {
			t.Errorf("Member(strs, %q) = %+v, want nil", name, m)
		}
	}
}

func TestGenerateEmbeds(t *testing.T) {
	ix, _ := generateLoaded(t)
	tests := []struct {
		pkg, typ, name string
		wantPkg        string
		wantRecv       string
		wantURL        string
	}{
		{"strs", "ReadCloser", "Read", "strs", "Reader", "https://pkg.go.dev/strs#Reader.Read"},
		{"strs", "ReadCloser", "Close", "strs", "Closer", "https://pkg.go.dev/strs#Closer.Close"},
		{"strs", "Pair", "WriteString", "strs", "Builder", "https://pkg.go.dev/strs#Builder.WriteString"},
		{"files", "File", "Read", "iox", "Reader", "https://pkg.go.dev/iox#Reader.Read"},
		{"files", "Error", "Error", "builtin", "error", "https://pkg.go.dev/builtin#error.Error"},
	}
	for _, tt := range tests {
		m := ix.Method(tt.pkg, tt.typ, tt.name)
		if m == nil {
			t.Errorf("Method(%s, %s, %s) = nil", tt.pkg, tt.typ, tt.name)
			continue
		}
		if m.Package().Path != tt.wantPkg || m.Recv != tt.wantRecv || m.URL() != tt.wantURL {
			t.Errorf("Method(%s, %s, %s) = %s.%s.%s at %s, want %s.%s.%s at %s", tt.pkg, tt.typ, tt.name,
				m.Package().Path, m.Recv, m.Name, m.URL(), tt.wantPkg, tt.wantRecv, tt.name, tt.wantURL)
		}
	}
}

// The index is committed; a regenerated file must only differ when the docs differ.
func TestGenerateIsDeterministic(t *testing.T) {
	_, a := generateLoaded(t)
	_, b := generateLoaded(t)
	if !bytes.Equal(a, b) {
		t.Error("2 runs over the same GOROOT gave different bytes")
	}
}

func TestRunWritesIndex(t *testing.T) {
	out := filepath.Join(t.TempDir(), "stdlib.json.gz")
	if err := run(config{goroot: writeFakeGOROOT(t), goos: "linux", goarch: "amd64"}, out, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	ix, err := godoc.LoadFile(out)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if ix.Member("strs", "Cut") == nil {
		t.Error("the written index has no strs.Cut")
	}
	entries, err := os.ReadDir(filepath.Dir(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("output directory has %d entries, want only the index (no temporary file)", len(entries))
	}
}

func TestGenerateErrors(t *testing.T) {
	if _, err := generate(config{goroot: t.TempDir(), goos: "linux", goarch: "amd64"}); err == nil {
		t.Error("generate without a VERSION file: err = nil")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("devel +abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(config{goroot: root, goos: "linux", goarch: "amd64"}); err == nil {
		t.Error("generate with a devel VERSION: err = nil")
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("go1.27.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(config{goroot: root, goos: "linux", goarch: "amd64"}); err == nil {
		t.Error("generate over an empty src: err = nil")
	}
}

func TestGenerateChecksRelease(t *testing.T) {
	root := writeFakeGOROOT(t) // go1.99.3
	for want, ok := range map[string]bool{"": true, "go1.99": true, "go1.99.3": true, "go1.9": false, "go1.98": false, "go1.99.4": false} {
		_, err := generate(config{goroot: root, goos: "linux", goarch: "amd64", want: want})
		if (err == nil) != ok {
			t.Errorf("generate with -go %q over go1.99.3: err = %v, want ok = %t", want, err, ok)
		}
	}
}

func TestSkipDir(t *testing.T) {
	tests := map[string]bool{
		"strings":           false,
		"net/http":          false,
		"runtime/debug":     false,
		"internal":          true,
		"crypto/internal":   true,
		"vendor":            true,
		"go/types/testdata": true,
		"cmd":               true,
		"syscall/js":        true,
		"runtime/cgo":       true,
		"simd":              true,
		".git":              true,
		"_obj":              true,
	}
	for dir, want := range tests {
		if got := skipDir(dir); got != want {
			t.Errorf("skipDir(%q) = %t, want %t", dir, got, want)
		}
	}
}

func TestOneLine(t *testing.T) {
	tests := map[string]string{
		"func F(a int) error":                              "func F(a int) error",
		"func F(\n\ta int,\n\tb string,\n) error":          "func F(a int, b string) error",
		"func F(a int,\n\tb string) error":                 "func F(a int, b string) error",
		"func F[\n\tK comparable,\n\tV any,\n](m map[K]V)": "func F[K comparable, V any](m map[K]V)",
		"[2]struct {\n\tSize uint32\n\tCount int\n}":       "[2]struct { Size uint32; Count int }",
		"~int | ~int8 |\n\t~string":                        "~int | ~int8 | ~string",
	}
	for in, want := range tests {
		if got := oneLine(in); got != want {
			t.Errorf("oneLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMinorVersion(t *testing.T) {
	tests := map[string]int{"go1.27.2": 27, "go1.27": 27, "go1.28rc1": 28, "go1.9": 9, "devel +abc": -1, "go2.0": -1, "": -1}
	for in, want := range tests {
		if got := minorVersion(in); got != want {
			t.Errorf("minorVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDefaultName(t *testing.T) {
	tests := map[string]string{"io": "io", "net/http": "http", "math/rand/v2": "rand", "v2": "v2", "crypto/x509/pkix": "pkix"}
	for in, want := range tests {
		if got := defaultName(in); got != want {
			t.Errorf("defaultName(%q) = %q, want %q", in, got, want)
		}
	}
}
