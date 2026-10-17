---
id: vet-buildtag
source: vet
analyzer: buildtag
match: ^misplaced (?://go:build|\+build) comment$
match: ^malformed //go:build line .+$
match: ^unexpected extra //go:build line$
match: ^possible malformed \+build comment$
match: ^\+build lines do not match //go:build condition$
match: ^invalid (?:double negative|non-alphanumeric) build constraint: .+$
match: ^invalid go version ".+" in build constraint$
title: A build constraint is misplaced or malformed
links:
- Command go: Build constraints | https://pkg.go.dev/cmd/go#hdr-Build_constraints
- The buildtag check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/buildtag
---
## What it means

The line at {{file}}:{{line}} is a build constraint, such as `//go:build linux`. A build constraint decides whether Go compiles the file at all. Go reads it only at the top of the file, before the `package` clause. In any other place it is a plain comment, so the file is built everywhere, whatever the line says.

## Why Go does this

The docs of the go command say that a constraint must appear near the top of the file, preceded only by blank lines and other comments, and followed by a blank line. The blank line keeps it apart from the package documentation. A constraint in the wrong place is not an error for the compiler, so `go vet` reports it. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and ask whether the file needs a constraint at all. Exercise files are built on every platform, so they do not. If you did not mean to limit the file, delete the line. If you did, move it to the very first line and put a blank line after it. `gofmt` moves a misplaced `//go:build` line to the top for you.

## Example

```go
package fixture

// After the package clause, Go ignores the next line.
//go:build linux
```

Put the constraint first, followed by a blank line:

```go
//go:build linux

package fixture
```

## Learn more
