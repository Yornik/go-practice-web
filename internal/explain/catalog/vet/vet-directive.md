---
id: vet-directive
source: vet
analyzer: directive
match: ^//go:debug directive only valid .+$
match: ^invalid space .+ in .+ directive$
title: A //go:debug directive is in a place where Go ignores it
links:
- Go, Backwards Compatibility, and GODEBUG | https://go.dev/doc/godebug#default
- The directive check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/directive
---
## What it means

The line at {{file}}:{{line}} is a `//go:debug` directive. It changes a GODEBUG setting, which switches an old or new behaviour of the runtime or the standard library for a program. Go reads it only at the top of a file of package main or a test file, before the `package` clause. Anywhere else the toolchain ignores the line, so the setting that you wanted does not apply.

## Why Go does this

A GODEBUG setting belongs to a whole program, so only the package that builds the program, or a test, may set it. The Go documentation on GODEBUG says that the toolchain ignores `//go:debug` lines in any other place and that `go vet` reports them as misplaced. `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and ask whether you need the setting at all. Exercise code never does: the tests run with the defaults of the course module. If a test of your own needs it, put the line at the very top of a `_test.go` file, before `package`, with a blank line after it. Otherwise delete the line.

## Example

```go
//go:debug panicnil=1

package fixture // not package main: the line is ignored
```

Delete the directive, and write code that works with the defaults:

```go
package fixture
```

## Learn more
