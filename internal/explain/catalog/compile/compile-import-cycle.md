---
id: compile-import-cycle
source: compile
match: (?s)^package (?P<pkg>\S+)\n.*: import cycle not allowed(?: in test)?$
title: The package {{pkg}} imports itself, directly or through other packages
links:
- The Go spec: Import declarations | https://go.dev/ref/spec#Import_declarations
- Package testing | https://pkg.go.dev/testing
- How to Write Go Code | https://go.dev/doc/code
---
## What it means

Following the imports of `{{pkg}}` leads back to `{{pkg}}`. The indented `imports` lines under the message show the path, 1 import per line, with the file that contains it. The last line names the import that closes the circle. `in test` means a test file of the package imports the package again, directly or through another package.

## Why Go does this

The spec forbids a package to import itself, directly or indirectly. Go compiles a package after everything it imports and initializes the imported packages first. In a cycle, neither package can come first. The rule also keeps the dependency graph free of cycles, so you can read it from top to bottom.

## How to find it

Read the `imports` lines and find the import that should not be there:

- A file imports its own package by path. Inside a package you use its names directly, without an import.
- A test file in `package name` imports `name`. Either use the names without the import, or make it an external test, `package name_test`, which may import the package. The course tests use that form.
- 2 of your own packages need each other: move the shared part into a third package that both import, or let 1 side depend on an interface instead.

## Example

```go
// x_test.go
package textutil

import (
	"testing"

	"example.com/textutil" // import cycle not allowed in test
)
```

Use the external test package, which may import the package it tests:

```go
// x_test.go
package textutil_test
```

## Learn more
