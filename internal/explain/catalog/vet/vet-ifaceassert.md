---
id: vet-ifaceassert
source: vet
analyzer: ifaceassert
match: ^impossible type assertion: no type can implement both (?P<have>.+) and (?P<want>.+) \(conflicting types for (?P<method>\w+) method\)$
title: No type can be both {{have}} and {{want}}, so this assertion always fails
links:
- The Go spec: Type assertions | https://go.dev/ref/spec#Type_assertions
- Effective Go: Interface conversions and type assertions | https://go.dev/doc/effective_go#interface_conversions
- The ifaceassert check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/ifaceassert
---
## What it means

The type assertion at {{file}}:{{line}} asks whether a value of the interface type `{{have}}` also implements `{{want}}`. Both interfaces have a method `{{method}}`, with different signatures. A type has only 1 method with a given name, so no value can implement both. The assertion always fails: the form with `, ok` gives false, and the form without it panics.

## Why Go does this

For an interface type, the Go spec says that `x.(T)` asserts that the dynamic type of `x` implements `T`. The compiler allows assertions between interfaces, because it cannot know every type that a value may hold. It does not check whether 2 interfaces can ever meet. `go vet` does, and `go test` runs this check before the tests.

## How to find it

Go to {{file}}:{{line}} and compare the 2 `{{method}}` methods in their declarations or docs. Most often you declared your own interface and gave the method a different signature from the standard one. Use the standard interface instead, such as `io.StringWriter`, or copy its exact signature.

## Example

```go
type StringWriter interface {
	Write(s string) error // io.Writer has Write([]byte) (int, error)
}

if sw, ok := w.(StringWriter); ok { // never true for an io.Writer
```

Use the interface that the standard library defines:

```go
if sw, ok := w.(io.StringWriter); ok {
	_, err := sw.WriteString(s)
	return err
}
```

## Learn more
