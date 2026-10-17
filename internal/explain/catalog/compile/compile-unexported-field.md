---
id: compile-unexported-field
source: compile
match: ^.+? undefined \(cannot refer to unexported (?:field|method|object) (?P<name>[^\s)]+)\)$
match: ^cannot refer to unexported field (?P<name>\S+) in struct literal of type .+$
title: {{name}} is unexported, so only its own package can use it
links:
- The Go spec: Exported identifiers | https://go.dev/ref/spec#Exported_identifiers
- Effective Go: Names | https://go.dev/doc/effective_go#names
- How to Write Go Code | https://go.dev/doc/code
---
## What it means

The type has a field or method `{{name}}`, but that type comes from another package and `{{name}}` starts with a lower-case letter. It is unexported: only code inside the package that declares it can read, write or call it. Your code at {{file}}:{{line}} is in a different package.

## Why Go does this

Go has 1 rule for visibility, and no keywords such as `public` or `private`: a name that starts with an upper-case letter is exported, every other name is private to its package. A package can then change its internal fields without breaking the code that uses it. The standard library hides most of the state of its types this way. For example, `time.Time` stores the time in unexported fields and offers methods to read it.

## How to find it

- Read the exported API of the type with `go doc`, for example `go doc time.Time`, or open its page on pkg.go.dev. There is almost always an exported method or function that gives you what you want.
- In a struct literal of another package's type, set only exported fields, or use the constructor the package offers.
- An external test package (`package name_test`), as the course uses, is another package too. Tests see only what your package exports.

## Example

```go
func Elapsed(start time.Time) int64 {
	return start.wall // start.wall undefined (cannot refer to unexported field wall)
}
```

Use the exported methods of the type:

```go
func Elapsed(start, now time.Time) time.Duration {
	return now.Sub(start)
}
```

## Learn more
