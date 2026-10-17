---
id: staticcheck-u1000
source: staticcheck
code: U1000
match: ^(?P<kind>\w+) (?P<name>.+) is unused$
title: The {{kind}} {{name}} is never used
links:
- staticcheck: the unused check, U1000 | https://pkg.go.dev/honnef.co/go/tools/unused
- Effective Go: Unused imports and variables | https://go.dev/doc/effective_go#blank_unused
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
---
## What it means

No code in the package uses the {{kind}} `{{name}}`. It is not exported, so no other package can use it either. A helper that you wrote but never call often points to an unfinished step: you meant to call it somewhere, and that code is missing or calls something else.

## Why Go does this

The compiler rejects unused imports and unused local variables, but it accepts unused functions, types, fields and package-level variables. staticcheck finds those. It counts uses in the test files of the package too. Exported names count as used, because another package may use them.

## How to find it

Search the package for `{{name}}`. Then ask:

- Should another function call it? Find the place, and check whether that place calls a different function by mistake.
- Is it a field that nothing reads or writes? Then the logic that should use it is missing, or the field is not needed.
- Is it left over from an earlier attempt? Delete it.

## Example

```go
func Normalize(s string) string { return strings.ToLower(s) } // the TrimSpace step is missing

func trimAll(xs []string) []string { ... } // never called
```

Call the helper where it belongs, or delete it:

```go
func Normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
```

## Learn more
