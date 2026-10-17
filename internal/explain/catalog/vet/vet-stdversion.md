---
id: vet-stdversion
source: vet
analyzer: stdversion
match: ^(?P<symbol>\S+) requires (?P<need>go[\d.]+) or later \((?P<scope>module|file) is (?P<have>go[\d.]+)\)$
title: {{symbol}} needs {{need}}, but this code must work with {{have}}
links:
- Go Modules Reference: The go directive | https://go.dev/ref/mod#go-mod-file-go
- Go Toolchains | https://go.dev/doc/toolchain
- The stdversion check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/stdversion
---
## What it means

`{{symbol}}` was added to the standard library in {{need}}. The `go` line in `go.mod` says {{have}}: the module promises to build with that release. The runner has a newer Go, so the code compiles there. But someone with Go {{have}} could not build it, so `go vet` reports it, and `go test` stops before the tests run.

## Why Go does this

Since Go 1.21, the `go` line in `go.mod` is the minimum Go release for the module. The compiler checks language features against it, but not every new function of the standard library. The stdversion check of `go vet` covers those. The documentation in the editor shows the standard library of the runner's newer release, so it also lists functions that the course module cannot use. pkg.go.dev marks such a symbol with the release that added it, such as `added in go1.25.0`.

## How to find it

Go to {{file}}:{{line}} and look up `{{symbol}}`. Find a way that works in {{have}}: often the docs describe the older approach, or the new function is a shortcut for a few lines that you can write yourself. The `go.mod` of the course is fixed, so changing the version is not an option.

## Example

```go
wg.Go(func() { handle(item) }) // sync.WaitGroup.Go needs go1.25
```

Write the 2 steps that Go does for you:

```go
wg.Add(1)
go func() {
	defer wg.Done()
	handle(item)
}()
```

## Learn more
