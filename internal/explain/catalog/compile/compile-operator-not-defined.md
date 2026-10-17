---
id: compile-operator-not-defined
source: compile
match: ^invalid operation: operator (?P<op>\S+) not defined on (?P<expr>.+?) \((?P<what>.+)\)$
match: ^invalid operation: (?P<expr>.+) \(operator (?P<op>\S+) not defined on (?P<what>.+)\)$
title: The operator {{op}} does not work on this type
links:
- The Go spec: Arithmetic operators | https://go.dev/ref/spec#Arithmetic_operators
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
- Package slices | https://pkg.go.dev/slices
---
## What it means

At {{file}}:{{line}} you applied `{{op}}` to `{{expr}}`, and the type ({{what}}) does not support that operator. Go has no operator overloading, so each operator works only on the built-in types the spec lists for it.

## Why Go does this

`+` works on numbers and strings, `-`, `*` and `/` on numbers, `!`, `&&` and `||` on booleans, and `<`, `<=`, `>` and `>=` on integers, floats and strings. Slices, maps and structs have no arithmetic and no order. A struct has no `<`, because Go cannot know which field decides the order; you must say it.

## How to find it

Decide what the operator should mean for this type, and write that out:

- Join 2 slices: `append(a, b...)`, or `slices.Concat(a, b)`.
- Merge 2 maps: a loop, or `maps.Copy(dst, src)`.
- Order 2 structs: compare the field that decides, such as `a.Start.Before(b.Start)` for times, or `cmp.Compare(a.N, b.N)`.
- A type parameter `T` that should support `<`: give it the constraint `cmp.Ordered`.
- `!` on a number: compare instead, `n == 0`.

## Example

```go
type Window struct {
	Start time.Time
	End   time.Time
}

func Before(a, b Window) bool {
	return a < b // invalid operation: a < b (operator < not defined on struct)
}
```

Compare the field that defines the order:

```go
	return a.Start.Before(b.Start)
```

## Learn more
