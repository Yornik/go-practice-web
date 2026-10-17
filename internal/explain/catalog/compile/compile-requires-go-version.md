---
id: compile-requires-go-version
source: compile
match: ^(?P<feature>.+) requires (?P<version>go1\.\d+) or later(?: \(-lang was set to (?P<lang>go1\.\d+); check go\.mod\))?$
title: This needs {{version}}, but the course compiles your code as Go 1.24
links:
- Go Modules Reference: The go directive | https://go.dev/ref/mod#go-mod-file-go
- The Go spec: Language versions | https://go.dev/ref/spec#Language_versions
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
---
## What it means

At {{file}}:{{line}} you used a language feature, `{{feature}}`, that Go added in {{version}}. The toolchain knows it, but the course module declares `go 1.24` in its `go.mod`, so the compiler accepts only the language of Go 1.24.

## Why Go does this

The `go` line in `go.mod` sets the language version of a module. A newer toolchain then compiles the module with the rules of that version, so code that says it is Go 1.24 also builds with a Go 1.24 toolchain. The course pins Go 1.24 so that every learner writes the same language. You cannot change its `go.mod` here.

## How to find it

Write the code the Go 1.24 way:

- `generic method`: a method cannot have its own type parameters in Go 1.24. Make it a generic function that takes the receiver as a parameter: `func First[T any](r *Ring) (T, bool)`. Or move the type parameter to the type.
- `use of promoted field ... in struct literal`: set the embedded struct as 1 field, `User{Base: Base{ID: 1}}`.
- `new(...)` with a value: declare a variable and take its address, `v := 42` and `&v`.

The spec marks each newer feature with its version, such as `[Go 1.27]`. Check that marker when you read the spec.

## Example

```go
func (r *Ring) First[T any]() (T, bool) { // generic method requires go1.27 or later
	...
}
```

Write a generic function that takes the receiver:

```go
func First[T any](r *Ring) (T, bool) {
	...
}
```

## Learn more
