---
id: vet-composites
source: vet
analyzer: composites
match: ^(?P<type>\S+) struct literal uses unkeyed fields$
title: The {{type}} literal lists its values without field names
links:
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- Go 1 and the Future of Go Programs | https://go.dev/doc/go1compat
- The composites check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/composite
---
## What it means

You build a `{{type}}` by listing values in the order of its fields, without field names. It compiles today. But `{{type}}` belongs to another package. When a new release of that package adds a field, your literal no longer compiles. When 2 fields of the same type swap places, it still compiles and puts your values in the wrong fields.

## Why Go does this

The Go spec allows a struct literal without keys only when it lists a value for every field, in the order of declaration. The Go 1 compatibility promise allows new fields in exported structs, and it names this case: unkeyed literals may fail to compile after such a change, keyed literals keep working. `go vet` reports unkeyed literals of types from other packages. For your own types the choice is yours.

## How to find it

Go to {{file}}:{{line}} and look up the fields of `{{type}}` in its docs. Write each value as `Field: value`. Fields that you leave out get their zero value, so you can drop zero values from the literal.

## Example

```go
return net.TCPAddr{net.IPv4(127, 0, 0, 1), port, ""}
```

Name every field that you set:

```go
return net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
```

## Learn more
