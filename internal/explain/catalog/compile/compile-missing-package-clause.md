---
id: compile-missing-package-clause
source: compile
match: ^expected 'package', found (?P<found>.+)$
title: The file does not start with a package clause
links:
- The Go spec: Package clause | https://go.dev/ref/spec#Package_clause
- The Go spec: Source file organization | https://go.dev/ref/spec#Source_file_organization
- How to Write Go Code | https://go.dev/doc/code
---
## What it means

The first word of code in {{file}} must be `package` followed by the package name, but Go found {{found}}. Comments may come before the package clause; nothing else may. `'EOF'` means the file is empty or holds only comments.

## Why Go does this

Every Go source file starts by saying which package it belongs to. Go needs that before it reads anything else: the imports and declarations of the file belong to that package. The go command reads the package clause of every file in a directory to decide which files form the package, so a file without one stops the whole build.

## How to find it

Open {{file}} and look at its first lines:

- The `package` line was deleted, or a paste replaced it. Add it back: `package` followed by the same name as the other files of the exercise.
- The file starts with an `import` or a function that was meant to go further down.
- A test file of the course uses `package name_test`. A stub file uses the name after the number of the exercise directory, for example `logfmt` for `01-logfmt`.

When you are not sure of the name, look at the first line of another file in the same exercise.

## Example

```go
import "strings" // expected 'package', found 'import'

func Upper(s string) string {
	return strings.ToUpper(s)
}
```

Start the file with its package clause:

```go
package textutil

import "strings"
```

## Learn more
