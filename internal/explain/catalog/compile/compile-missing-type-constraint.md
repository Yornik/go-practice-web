---
id: compile-missing-type-constraint
source: compile
match: ^(?:syntax error: )?missing type constraint$
match: ^undefined array length \S+ or missing type constraint$
title: Every type parameter needs a constraint
links:
- The Go spec: Type parameter declarations | https://go.dev/ref/spec#Type_parameter_declarations
- The Go spec: Type constraints | https://go.dev/ref/spec#Type_constraints
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

The type parameter list at {{file}}:{{line}} names a type parameter without a constraint, as in `[T]` or `[K, V]`. In Go, every type parameter needs a constraint after its name, even when any type is allowed.

## Why Go does this

The constraint says which type arguments a caller may pass and which operations the generic code may use. Go wants that written down, and `any` is the explicit way to say "every type". For a generic type with 1 parameter, `type Ring[T] struct` also looks like an array type, `Ring` of length `T`, so the compiler says `undefined array length T or missing type constraint`.

## How to find it

Go to {{file}}:{{line}} and add a constraint to each name in the square brackets:

- Any type: `[T any]`.
- Map keys or `==`: `[K comparable]`.
- Ordering with `<`: `[T cmp.Ordered]`.
- Several parameters with the same constraint share it: `[K, V any]` gives both `any`.

The constraints belong only to the declaration. A method of a generic type names the parameters without constraints: `func (r *Ring[T]) Len() int`.

## Example

```go
func Reverse[T](xs []T) []T { // syntax error: missing type constraint
	out := make([]T, 0, len(xs))
	for i := len(xs) - 1; i >= 0; i-- {
		out = append(out, xs[i])
	}
	return out
}
```

Add the constraint `any`:

```go
func Reverse[T any](xs []T) []T {
```

## Learn more
