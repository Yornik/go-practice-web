// Command gendocs indexes the standard library of a Go installation for the editor of go-practice-web: hover
// docs, completion after "pkg." and signature help. internal/godoc embeds the result and serves it.
//
// Run it with the Go release of the runner image, so the docs describe the compiler that runs the learner's code:
//
//	GOTOOLCHAIN=go1.27.2 go generate ./internal/godoc
//
// The go:generate line passes -go with that release, so a run with another release fails instead of writing
// docs for the wrong compiler. Update it together with the Go version in Dockerfile.runner.
//
// It selects files with go/build for linux/amd64 (the runner's platform), reads them with go/parser and go/doc,
// and writes gzip-compressed JSON in the format of internal/godoc.
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/yornik/go-practice-web/internal/godoc"
)

// excluded lists packages that learners cannot or should not import. Everything below them is skipped too.
// Directories named internal, vendor and testdata, and the cmd tree, are skipped by rule (see skipDir).
var excluded = map[string]bool{
	"arena":        true, // a GOEXPERIMENT that the runner does not enable
	"runtime/asan": true, // sanitizer glue; programs do not import it
	"runtime/cgo":  true, // cgo glue; programs do not import it
	"runtime/msan": true, // sanitizer glue; programs do not import it
	"runtime/race": true, // race detector glue; -race links it without an import
	"simd":         true, // experimental intrinsics behind a GOEXPERIMENT, and thousands of members
	"syscall/js":   true, // WebAssembly only; the runner is linux/amd64
}

const (
	// maxInline is the longest struct or interface body printed on the signature line. Longer bodies become
	// "struct{ ... }", and their fields and methods are members of their own.
	maxInline = 100
	// maxIfaceLines is the most methods an interface signature lists. Small interfaces such as io.Reader or
	// context.Context are defined by their methods, so the hover shows them.
	maxIfaceLines = 12
	// maxValue is the longest const or var value kept in a signature.
	maxValue = 80
	// maxExtension is how much of a code block or list docText adds to a first paragraph that ends in ":".
	maxExtension = 600
)

var printConfig = printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}

type config struct {
	goroot string
	goos   string
	goarch string
	want   string // the Go release the runner image uses, such as "go1.27"; empty accepts any
}

func main() {
	var cfg config
	flag.StringVar(&cfg.goroot, "goroot", build.Default.GOROOT, "the Go installation to index")
	flag.StringVar(&cfg.goos, "goos", "linux", "GOOS for build constraints: the runner's")
	flag.StringVar(&cfg.goarch, "goarch", "amd64", "GOARCH for build constraints: the runner's")
	flag.StringVar(&cfg.want, "go", "", "fail unless the GOROOT is this Go release, such as go1.27 (the runner image's)")
	out := flag.String("o", "internal/godoc/data/stdlib.json.gz", "the output file")
	verbose := flag.Bool("v", false, "list the largest packages")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("gendocs: ")
	if err := run(cfg, *out, *verbose); err != nil {
		log.Fatal(err)
	}
}

func run(cfg config, out string, verbose bool) error {
	ix, err := generate(cfg)
	if err != nil {
		return err
	}
	if minorVersion(runtime.Version()) != minorVersion(ix.GoVersion) {
		// GOEXPERIMENT build tags come from the running toolchain, so a mismatch can drop or add files.
		log.Printf("warning: running %s but indexing %s; run with GOTOOLCHAIN=%s", runtime.Version(), ix.GoVersion, ix.GoVersion)
	}
	var gz bytes.Buffer
	if err := godoc.Encode(&gz, ix); err != nil {
		return err
	}
	raw, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(out, gz.Bytes()); err != nil {
		return err
	}
	members := 0
	for _, p := range ix.Packages {
		members += len(p.Members)
	}
	log.Printf("%s %s/%s: %d packages, %d members, %.1f MB JSON, %.2f MB gzipped, written to %s",
		ix.GoVersion, ix.GOOS, ix.GOARCH, len(ix.Packages), members, mb(len(raw)), mb(gz.Len()), out)
	if verbose {
		printLargest(ix, 20)
	}
	return nil
}

func mb(n int) float64 { return float64(n) / (1 << 20) }

func printLargest(ix *godoc.Index, n int) {
	type size struct {
		path  string
		bytes int
	}
	var sizes []size
	for _, p := range ix.Packages {
		b, _ := json.Marshal(p)
		sizes = append(sizes, size{p.Path, len(b)})
	}
	slices.SortFunc(sizes, func(a, b size) int { return b.bytes - a.bytes })
	for _, s := range sizes[:min(n, len(sizes))] {
		log.Printf("  %-24s %6.0f KB JSON", s.path, float64(s.bytes)/1024)
	}
}

// writeFileAtomic replaces name in 1 step, so a failed run never leaves a truncated index behind.
func writeFileAtomic(name string, data []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(name), ".gendocs-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp) // best effort: the error that matters is already in err
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(0o644)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

// generate indexes every package under cfg.goroot/src.
func generate(cfg config) (*godoc.Index, error) {
	version, err := readVersion(cfg.goroot)
	if err != nil {
		return nil, err
	}
	// Docs from another release would offer the learner functions that the runner's compiler does not have.
	if cfg.want != "" && version != cfg.want && !strings.HasPrefix(version, cfg.want+".") {
		return nil, fmt.Errorf("%s is %s, but the runner uses %s; run it with that release, for example GOTOOLCHAIN=%s.0 go generate ./internal/godoc",
			cfg.goroot, version, cfg.want, cfg.want)
	}
	ctx := buildContext(cfg, version)
	src := filepath.Join(cfg.goroot, "src")
	ix := &godoc.Index{GoVersion: version, GOOS: cfg.goos, GOARCH: cfg.goarch}
	err = filepath.WalkDir(src, func(dir string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || dir == src {
			return nil
		}
		rel, err := filepath.Rel(src, dir)
		if err != nil {
			return err
		}
		importPath := filepath.ToSlash(rel)
		if skipDir(importPath) {
			return filepath.SkipDir
		}
		p, err := indexPackage(ctx, dir, importPath)
		if err != nil {
			return fmt.Errorf("%s: %w", importPath, err)
		}
		if p != nil {
			ix.Packages = append(ix.Packages, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(ix.Packages) == 0 {
		return nil, fmt.Errorf("no packages found in %s", src)
	}
	return ix, nil
}

func skipDir(importPath string) bool {
	name := path.Base(importPath)
	switch {
	case name == "internal" || name == "vendor" || name == "testdata":
		return true
	case strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_"):
		return true // the go command ignores these too
	case importPath == "cmd":
		return true
	}
	return excluded[importPath]
}

// buildContext selects the files the runner compiles: its platform, cgo on (the race detector needs it), and the
// release tags of the indexed Go release, not of the Go that runs this program.
func buildContext(cfg config, version string) *build.Context {
	ctx := build.Default
	ctx.GOROOT = cfg.goroot
	ctx.GOPATH = ""
	ctx.GOOS = cfg.goos
	ctx.GOARCH = cfg.goarch
	ctx.CgoEnabled = true
	ctx.BuildTags = nil
	ctx.ToolTags = slices.Clone(build.Default.ToolTags)
	ctx.ReleaseTags = nil
	for i := 1; i <= minorVersion(version); i++ {
		ctx.ReleaseTags = append(ctx.ReleaseTags, "go1."+strconv.Itoa(i))
	}
	return &ctx
}

func readVersion(goroot string) (string, error) {
	b, err := os.ReadFile(filepath.Join(goroot, "VERSION"))
	if err != nil {
		return "", fmt.Errorf("read the Go version of %s: %w", goroot, err)
	}
	v, _, _ := strings.Cut(string(b), "\n")
	v = strings.TrimSpace(v)
	if minorVersion(v) < 0 {
		return "", fmt.Errorf("%s/VERSION says %q; want a release such as go1.27.2", goroot, v)
	}
	return v, nil
}

// minorVersion returns 27 for "go1.27.2" and "go1.27rc1", or -1.
func minorVersion(v string) int {
	rest, ok := strings.CutPrefix(v, "go1.")
	if !ok {
		return -1
	}
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return -1
	}
	return n
}

// indexPackage returns nil for a directory without an importable package for the target platform.
func indexPackage(ctx *build.Context, dir, importPath string) (*godoc.Package, error) {
	bp, err := ctx.ImportDir(dir, 0)
	var noGo *build.NoGoError
	if errors.As(err, &noGo) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if bp.Name == "main" {
		return nil, nil
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range slices.Concat(bp.GoFiles, bp.CgoFiles) {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	var mode doc.Mode
	if importPath == "builtin" {
		mode = doc.AllDecls // the predeclared identifiers are lower case
	}
	dp, err := doc.NewFromFiles(fset, files, importPath, mode)
	if err != nil {
		return nil, err
	}
	g := &pkgGen{
		fset:    fset,
		dp:      dp,
		parser:  dp.Parser(),
		imports: importNames(files),
		seen:    make(map[string]bool),
	}
	p := g.build()
	if g.err != nil {
		return nil, g.err
	}
	return p, nil
}

// importNames maps the names that files use for their imports to the import paths, to resolve embedded types
// such as io.Reader.
func importNames(files []*ast.File) map[string]string {
	m := make(map[string]string)
	for _, f := range files {
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			name := defaultName(p)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name != "_" && name != "." {
				m[name] = p
			}
		}
	}
	return m
}

// defaultName is the package name an import path gets without a rename: "rand" for "math/rand/v2".
func defaultName(importPath string) string {
	dir, last := path.Split(importPath)
	if len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" && dir != "" {
		return path.Base(dir)
	}
	return last
}

type pkgGen struct {
	fset    *token.FileSet
	dp      *doc.Package
	parser  *comment.Parser
	imports map[string]string
	members []godoc.Member
	seen    map[string]bool // "Recv.Name" of the members added so far
	err     error
}

func (g *pkgGen) build() *godoc.Package {
	dp := g.dp
	p := &godoc.Package{Path: dp.ImportPath, Name: dp.Name, Synopsis: dp.Synopsis(dp.Doc)}
	p.Doc, p.Deprecated = g.docText(dp.Doc)
	g.values(dp.Consts, godoc.KindConst)
	g.values(dp.Vars, godoc.KindVar)
	g.funcs(dp.Funcs)
	for _, t := range dp.Types {
		g.typ(t)
		g.values(t.Consts, godoc.KindConst)
		g.values(t.Vars, godoc.KindVar)
		g.funcs(t.Funcs)
		// Methods come after the fields: a field shadows a method promoted from a deeper embedded type.
		for _, f := range t.Methods {
			d, dep := g.docText(f.Doc)
			g.add(godoc.Member{Name: f.Name, Kind: godoc.KindMethod, Recv: t.Name, Signature: g.funcSig(f.Decl), Doc: d, Deprecated: dep})
		}
	}
	p.Members = g.members
	if p.Members == nil {
		p.Members = []godoc.Member{} // "members": [] for a package such as time/tzdata, which only has a doc
	}
	return p
}

func (g *pkgGen) add(m godoc.Member) {
	if !g.exported(m.Name) {
		return
	}
	key := m.Recv + "." + m.Name
	if g.seen[key] {
		return
	}
	g.seen[key] = true
	g.members = append(g.members, m)
}

func (g *pkgGen) exported(name string) bool {
	return name != "_" && (token.IsExported(name) || g.dp.ImportPath == "builtin")
}

func (g *pkgGen) funcs(fs []*doc.Func) {
	for _, f := range fs {
		d, dep := g.docText(f.Doc)
		g.add(godoc.Member{Name: f.Name, Kind: godoc.KindFunc, Signature: g.funcSig(f.Decl), Doc: d, Deprecated: dep})
	}
}

func (g *pkgGen) funcSig(decl *ast.FuncDecl) string {
	d := *decl
	d.Doc, d.Body = nil, nil
	return oneLine(g.print(&d))
}

func (g *pkgGen) values(vs []*doc.Value, kind godoc.Kind) {
	for _, v := range vs {
		groupDoc, groupDep := g.docText(v.Doc)
		// In a const group, a spec without type and values repeats the previous type (and iota expression).
		var carried ast.Expr
		for _, s := range v.Decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typ := spec.Type
			if kind == godoc.KindConst {
				if typ != nil || len(spec.Values) > 0 {
					carried = typ
				} else {
					typ = carried
				}
			}
			d, dep := g.docText(spec.Doc.Text())
			d, dep = cmp.Or(d, groupDoc), cmp.Or(dep, groupDep)
			// A line comment is the most specific text in a group ("// open the file read-only."), but on a
			// single declaration it only repeats the doc comment above it.
			if line, _ := g.docText(spec.Comment.Text()); line != "" && (d == "" || len(v.Decl.Specs) > 1) {
				d = strings.TrimSpace(sentence(line) + " " + d)
			}
			for i, name := range spec.Names {
				sig := string(kind) + " " + name.Name
				if typ != nil {
					sig += " " + oneLine(g.print(typ))
				}
				if len(spec.Values) == len(spec.Names) {
					if val := g.valueText(spec.Values[i]); val != "" {
						sig += " = " + val
					} else if typ == nil {
						sig += " = ..."
					}
				}
				g.add(godoc.Member{Name: name.Name, Kind: kind, Signature: sig, Doc: d, Deprecated: dep})
			}
		}
	}
}

// valueText returns a value short enough for a hover, an elided call, literal or func, or "".
func (g *pkgGen) valueText(e ast.Expr) string {
	s := g.print(e)
	if !strings.Contains(s, "`") { // joining the lines of a raw string would change its value
		s = oneLine(s)
	}
	if fits(s, maxValue) {
		return s
	}
	switch x := e.(type) {
	case *ast.CallExpr:
		if f := oneLine(g.print(x.Fun)); fits(f, maxValue) {
			return f + "(...)"
		}
	case *ast.CompositeLit:
		if x.Type != nil {
			if t := oneLine(g.print(x.Type)); fits(t, maxValue) {
				return t + "{...}"
			}
		}
	case *ast.FuncLit:
		if t := oneLine(g.print(x.Type)); fits(t, maxValue) {
			return t + " {...}"
		}
	}
	return ""
}

func fits(s string, n int) bool { return len(s) <= n && !strings.Contains(s, "\n") }

func (g *pkgGen) typ(t *doc.Type) {
	var spec *ast.TypeSpec
	for _, s := range t.Decl.Specs {
		if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == t.Name {
			spec = ts
		}
	}
	if spec == nil {
		return
	}
	d, dep := g.docText(t.Doc)
	m := godoc.Member{Name: t.Name, Kind: godoc.KindType, Signature: g.typeSig(spec), Doc: d, Deprecated: dep}
	switch x := spec.Type.(type) {
	case *ast.StructType:
		m.Embeds = g.embeds(t.Name, x.Fields)
		g.add(m)
		g.fields(t.Name, x)
	case *ast.InterfaceType:
		m.Embeds = g.embeds(t.Name, x.Methods)
		g.add(m)
		g.interfaceMethods(t.Name, x)
	default:
		g.add(m)
	}
}

func (g *pkgGen) typeSig(spec *ast.TypeSpec) string {
	head := "type " + spec.Name.Name + g.typeParams(spec.TypeParams) + " "
	if spec.Assign.IsValid() {
		head += "= "
	}
	switch x := spec.Type.(type) {
	case *ast.StructType:
		return head + g.structBody(x)
	case *ast.InterfaceType:
		return head + g.interfaceBody(x)
	}
	return head + oneLine(g.print(spec.Type))
}

func (g *pkgGen) typeParams(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	// The printer has no entry point for a bare field list; a func type around it prints "func[T any]()".
	s := oneLine(g.print(&ast.FuncType{TypeParams: fl, Params: &ast.FieldList{}}))
	return strings.TrimSuffix(strings.TrimPrefix(s, "func"), "()")
}

func (g *pkgGen) structBody(st *ast.StructType) string {
	var entries []string
	for _, f := range st.Fields.List {
		typ := oneLine(g.print(f.Type))
		if len(f.Names) == 0 {
			entries = append(entries, typ)
			continue
		}
		var names []string
		for _, n := range f.Names {
			if g.exported(n.Name) {
				names = append(names, n.Name)
			}
		}
		if len(names) > 0 {
			entries = append(entries, strings.Join(names, ", ")+" "+typ)
		}
	}
	if len(entries) == 0 && !st.Incomplete {
		return "struct{}"
	}
	n := len(entries)
	if st.Incomplete {
		// Tells the learner that the type has state they cannot touch, such as strings.Builder.
		entries = append(entries, "/* unexported fields */")
	}
	s := "struct{ " + strings.Join(entries, "; ") + " }"
	if n <= 4 && len(s) <= maxInline {
		return s
	}
	return "struct{ ... }"
}

func (g *pkgGen) interfaceBody(it *ast.InterfaceType) string {
	var entries []string
	for _, f := range it.Methods.List {
		if len(f.Names) == 0 {
			entries = append(entries, oneLine(g.print(f.Type))) // an embedded interface or a type set
			continue
		}
		for _, n := range f.Names {
			if g.exported(n.Name) {
				entries = append(entries, n.Name+g.methodSig(f.Type))
			}
		}
	}
	if len(entries) == 0 && !it.Incomplete {
		return "interface{}"
	}
	if len(entries) <= 1 && !it.Incomplete {
		if s := "interface{ " + strings.Join(entries, "") + " }"; len(s) <= maxInline {
			return s
		}
	}
	if len(entries) > maxIfaceLines {
		return "interface{ ... }"
	}
	if it.Incomplete {
		entries = append(entries, "// contains filtered or unexported methods")
	}
	return "interface {\n\t" + strings.Join(entries, "\n\t") + "\n}"
}

// methodSig prints a method of an interface without its name: "(p []byte) (n int, err error)".
func (g *pkgGen) methodSig(typ ast.Expr) string {
	return strings.TrimPrefix(oneLine(g.print(typ)), "func")
}

func (g *pkgGen) fields(typeName string, st *ast.StructType) {
	for _, f := range st.Fields.List {
		d, dep := g.docText(commentText(f.Doc, f.Comment))
		typ := oneLine(g.print(f.Type))
		if len(f.Names) == 0 {
			if name := baseTypeName(f.Type); name != "" {
				g.add(godoc.Member{Name: name, Kind: godoc.KindField, Recv: typeName, Signature: typ, Doc: d, Deprecated: dep})
			}
			continue
		}
		for _, n := range f.Names {
			g.add(godoc.Member{Name: n.Name, Kind: godoc.KindField, Recv: typeName, Signature: n.Name + " " + typ, Doc: d, Deprecated: dep})
		}
	}
}

func (g *pkgGen) interfaceMethods(typeName string, it *ast.InterfaceType) {
	for _, f := range it.Methods.List {
		d, dep := g.docText(commentText(f.Doc, f.Comment))
		for _, n := range f.Names {
			g.add(godoc.Member{Name: n.Name, Kind: godoc.KindMethod, Recv: typeName, Signature: n.Name + g.methodSig(f.Type), Doc: d, Deprecated: dep})
		}
	}
}

// embeds lists the embedded types that can promote methods or fields, in the form godoc.Member.Embeds documents.
// go/doc already promotes the methods of unexported embedded types, so those are left out.
func (g *pkgGen) embeds(typeName string, fl *ast.FieldList) []string {
	var out []string
	for _, f := range fl.List {
		if len(f.Names) > 0 {
			continue
		}
		if ref := g.embedRef(f.Type); ref != "" && ref != typeName {
			out = append(out, ref)
		}
	}
	return out
}

func (g *pkgGen) embedRef(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return g.embedRef(x.X)
	case *ast.IndexExpr:
		return g.embedRef(x.X)
	case *ast.IndexListExpr:
		return g.embedRef(x.X)
	case *ast.Ident:
		switch {
		case g.exported(x.Name):
			return x.Name
		case x.Name == "error":
			return "builtin.error" // net.Error embeds error, so err.Error() resolves through builtin
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && g.imports[id.Name] != "" {
			return g.imports[id.Name] + "." + x.Sel.Name
		}
	}
	return ""
}

// baseTypeName is the field name of an embedded field: "Reader" for *Reader, io.Reader and Reader[T].
func baseTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return baseTypeName(x.X)
	case *ast.IndexExpr:
		return baseTypeName(x.X)
	case *ast.IndexListExpr:
		return baseTypeName(x.X)
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.Ident:
		return x.Name
	}
	return ""
}

func commentText(doc, line *ast.CommentGroup) string {
	if doc != nil {
		return doc.Text()
	}
	return line.Text()
}

// docText returns the first paragraph of a doc comment and the text of its "Deprecated:" paragraph, both as
// plain text on 1 line. The editor links to pkg.go.dev for the rest.
func (g *pkgGen) docText(text string) (first, deprecated string) {
	if strings.TrimSpace(text) == "" {
		return "", ""
	}
	blocks := g.parser.Parse(text).Content
	for i, b := range blocks {
		para, ok := b.(*comment.Paragraph)
		if !ok {
			continue
		}
		s := plainText(para.Text)
		if rest, ok := strings.CutPrefix(s, "Deprecated:"); ok {
			if deprecated == "" {
				deprecated = cmp.Or(strings.TrimSpace(rest), "Deprecated.")
			}
			continue
		}
		if first != "" {
			continue
		}
		first = s
		// "Compare returns" and "len returns the length of v, according to its type:" are only half a
		// sentence; the code block or list after them holds the answer.
		if !endsSentence(first) && i+1 < len(blocks) {
			if next := blockText(blocks[i+1], maxExtension); next != "" {
				first = strings.TrimSuffix(first, ":") + ": " + next
			}
		}
	}
	return first, deprecated
}

// blockText flattens a code block or list to 1 line of at most about limit bytes. It ends in "…" when it
// leaves lines or list items out.
func blockText(b comment.Block, limit int) string {
	var parts []string
	switch b := b.(type) {
	case *comment.Code:
		for l := range strings.SplitSeq(b.Text, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				parts = append(parts, l)
			}
		}
	case *comment.List:
		for _, item := range b.Items {
			for _, c := range item.Content {
				if p, ok := c.(*comment.Paragraph); ok {
					parts = append(parts, plainText(p.Text))
				}
			}
		}
	}
	var s strings.Builder
	for i, p := range parts {
		if i > 0 && s.Len()+len(p) > limit {
			s.WriteString(" …")
			break
		}
		if i > 0 {
			if strings.ContainsAny(s.String()[s.Len()-1:], ".,;:!?") {
				s.WriteString(" ")
			} else {
				s.WriteString("; ")
			}
		}
		s.WriteString(p)
	}
	return s.String()
}

func endsSentence(s string) bool {
	return s != "" && strings.ContainsAny(s[len(s)-1:], ".!?)\"")
}

// sentence ends s with a period, so it reads well in front of more text.
func sentence(s string) string {
	if s == "" || strings.ContainsAny(s[len(s)-1:], ".!?") {
		return s
	}
	return s + "."
}

func plainText(ts []comment.Text) string {
	var b strings.Builder
	var walk func([]comment.Text)
	walk = func(ts []comment.Text) {
		for _, t := range ts {
			switch t := t.(type) {
			case comment.Plain:
				b.WriteString(string(t))
			case comment.Italic:
				b.WriteString(string(t))
			case *comment.Link:
				walk(t.Text)
			case *comment.DocLink:
				walk(t.Text)
			}
		}
	}
	walk(ts)
	return strings.Join(strings.Fields(b.String()), " ")
}

// print formats n like gofmt, without the comments of nested fields: the printer keeps those, and a "//"
// comment would swallow the rest of a signature once oneLine joins its lines.
func (g *pkgGen) print(n ast.Node) string {
	ast.Inspect(n, func(n ast.Node) bool {
		if f, ok := n.(*ast.Field); ok {
			f.Doc, f.Comment = nil, nil
		}
		return true
	})
	var b strings.Builder
	if err := printConfig.Fprint(&b, g.fset, n); err != nil && g.err == nil {
		g.err = err
	}
	return b.String()
}

// tidyJoined removes the spaces that joining "func F(\n\ta int,\n) error" leaves inside the brackets.
var tidyJoined = strings.NewReplacer("( ", "(", ", )", ")", "[ ", "[", ", ]", "]")

// oneLine joins the lines that the printer keeps from the source, such as 1 parameter per line or the fields of
// a nested struct. A line break becomes "; " where Go's automatic semicolon rule would insert one.
func oneLine(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	prev := ""
	for l := range strings.SplitSeq(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if prev != "" {
			if endsStatement(prev) && !strings.ContainsAny(l[:1], ")]}") {
				b.WriteString("; ")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(l)
		prev = l
	}
	return tidyJoined.Replace(b.String())
}

// endsStatement reports whether the Go lexer would insert a semicolon after line: it ends in an identifier, a
// literal or a closing bracket (the spec's other cases cannot end a declaration line).
func endsStatement(line string) bool {
	c := line[len(line)-1]
	return c == '_' || c == ')' || c == ']' || c == '}' || c == '"' || c == '\'' || c == '`' ||
		'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c >= 0x80
}
