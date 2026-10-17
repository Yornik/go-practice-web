---
id: compile-package-mismatch
source: compile
match: ^found packages (?P<first>\S+) \((?P<firstfile>[^)]+)\) and (?P<second>\S+) \((?P<secondfile>[^)]+)\) in .+$
title: The files of 1 directory declare 2 packages: {{first}} and {{second}}
links:
- The Go spec: Package clause | https://go.dev/ref/spec#Package_clause
- How to Write Go Code | https://go.dev/doc/code
- Package testing | https://pkg.go.dev/testing
---
## What it means

All `.go` files in 1 directory must belong to the same package. Here `{{firstfile}}` says `package {{first}}` and `{{secondfile}}` says `package {{second}}`, so the go command cannot build the directory at all.

## Why Go does this

In Go, a directory is a package: the go command compiles all files of a directory together, and other code imports that directory by its path. 2 package names in 1 directory would make 2 packages at 1 import path. The only exception is a test file that uses the package name with `_test` appended, such as `package logfmt_test`. That is an external test package, which sees only the exported names, and the message shows it without the suffix.

## How to find it

Compare the first line of the 2 files the message names:

- A stub file must keep the package name it came with, such as `package logfmt`. Do not change it to `package main`; the exercise is a library, and the tests import it.
- A test file uses either the same name or the name with `_test`.
- A file that does not belong to this package must move to its own directory.

## Example

```go
// x.go
package textutil

// y.go, in the same directory
package main // found packages textutil (x.go) and main (y.go)
```

Give every file of the directory the same package clause:

```go
// y.go
package textutil
```

## Learn more
