---
id: compile-type-parameter-operator
source: compile
match: ^invalid operation: (?P<expr>.+) \(type parameter (?P<param>\S+) cannot use operator (?P<op>\S+)\)$
title: The constraint of {{param}} does not allow the operator {{op}}
links:
- The Go spec: Type constraints | https://go.dev/ref/spec#Type_constraints
- Package cmp: Ordered | https://pkg.go.dev/cmp#Ordered
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

The expression `{{expr}}` at {{file}}:{{line}} uses `{{op}}` on values of the type parameter `{{param}}`. Inside generic code, an operator is allowed only when every type in the constraint of `{{param}}` supports it. The constraint you gave allows types that do not.

## Why Go does this

Go type-checks a generic function once, for every possible type argument. With the constraint `any`, `{{param}}` could be a struct or a slice, which have no `<`. `comparable` allows `==` and `!=`, but not `<` or `+`. The constraint is the contract: it tells callers which types they may pass, and it tells the body which operations it may use.

## How to find it

Decide which operations the code needs, then pick the constraint that provides them:

- `<`, `<=`, `>`, `>=`: `cmp.Ordered`, which covers the integer, float and string types.
- `==` and `!=` only: `comparable`.
- `+`, `-`, `*`, `/` on numbers: a union of the types you support, such as `~int | ~int64 | ~float64`.
- An operation that does not fit a constraint, such as ordering structs: take a function as a parameter, for example `less func(a, b T) bool` or `cmp func(a, b T) int`.

## Example

```go
func Largest[T any](xs []T) T {
	var best T
	for i, x := range xs {
		if i == 0 || x > best { // invalid operation: x > best (type parameter T cannot use operator >)
			best = x
		}
	}
	return best
}
```

Constrain `T` to the types that have an order:

```go
func Largest[T cmp.Ordered](xs []T) T {
```

## Learn more
