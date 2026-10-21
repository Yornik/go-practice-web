---
id: compile-invalid-composite-literal
source: compile
match: ^invalid composite literal type (?P<type>[^(\n]+?)(?: \((?P<reason>[^)]+)\))?$
title: {{type}} cannot be built with a composite literal
course: 34-cache
links:
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}} you wrote `{{type}}{...}`, but a composite literal only works for structs, arrays, slices and maps. `{{type}}` is another kind of type: a number, a string, a function, a pointer or a type parameter. For a type parameter the message adds `no common underlying type`: the types in its constraint are not all the same kind of composite type.

## Why Go does this

A composite literal lists the elements or fields of a value, so the type must have elements or fields. A type parameter such as `T` constrained by `any` may be `int` or `string`, which have neither, so Go cannot know what `T{}` would mean.

## How to find it

Decide which value you want:

- The zero value of any type, a type parameter included: `var zero T`.
- A conversion of a value: `T(x)`, not `T{x}`.
- A new value behind a pointer: `new(T)` or `&S{}` for a struct type `S`.
- A struct: check that the name refers to the struct type and not to a pointer type or a function.

## Example

```go
func Zero[T any]() T {
	return T{} // invalid composite literal type T (no common underlying type)
}
```

Declare a variable; it starts as the zero value of `T`:

```go
func Zero[T any]() T {
	var zero T
	return zero
}
```

## Learn more

Exercise 09 practises the zero value of a type parameter.
