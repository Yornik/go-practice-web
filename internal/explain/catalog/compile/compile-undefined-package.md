---
id: compile-undefined-package
source: compile
match: ^undefined: (?P<name>bufio|bytes|cmp|context|errors|filepath|fmt|heap|http|io|iter|json|log|maps|math|netip|os|rand|reflect|regexp|runtime|slices|slog|sort|strconv|strings|sync|atomic|testing|time|unicode|utf8)$
title: The package {{name}} is not imported
links:
- The Go spec: Import declarations | https://go.dev/ref/spec#Import_declarations
- The Go standard library | https://pkg.go.dev/std
- Effective Go: Unused imports and variables | https://go.dev/doc/effective_go#blank_unused
---
## What it means

`{{name}}` is the name of a standard library package, and the file does not import it. A file sees a package only through its own import block, so `{{name}}.Something` at {{file}}:{{line}} has nothing to refer to.

## Why Go does this

Imports belong to 1 file, not to the package. Another file of the exercise can import `{{name}}` and this one still cannot use it. Go also refuses an import that a file does not use, so imports move with the code that needs them.

## How to find it

1. Add the package to the import block at the top of {{file}}. Format, or Shift+Alt+F, sorts the block for you.
2. Some packages sit in a directory: `json` is `encoding/json`, `http` is `net/http`, `atomic` is `sync/atomic`, `filepath` is `path/filepath`, `utf8` is `unicode/utf8`, `slog` is `log/slog`. `rand` is `math/rand/v2` in new code. Import the full path; you use the last part.
3. Did you mean a variable of your own? Then its declaration is missing or out of scope.

## Example

```go
package logfmt

func Format(k, v string) string {
	return fmt.Sprintf("%s=%q", k, v) // undefined: fmt
}
```

Import the package in the same file:

```go
package logfmt

import "fmt"

func Format(k, v string) string {
	return fmt.Sprintf("%s=%q", k, v)
}
```

## Learn more
