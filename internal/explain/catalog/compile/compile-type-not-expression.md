---
id: compile-type-not-expression
source: compile
match: ^(?P<name>.+) \(type\) is not an expression$
title: {{name}} is a type, but Go needs a value here
links:
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- The Go spec: Conversions | https://go.dev/ref/spec#Conversions
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
---
## What it means

At {{file}}:{{line}} you wrote `{{name}}`, which names a type, in a place that needs a value. A type describes values, but it is not a value itself. You cannot return it, assign it or pass it as an argument.

## Why Go does this

Go keeps types and values apart. A type can appear in a declaration, in a conversion `T(x)`, in a composite literal `T{...}`, in a type assertion `x.(T)`, or as the first argument of `new` and `make`. Everywhere else Go expects an expression with a value. This also holds for a generic type with its type arguments, such as `Index[string, int]`: that is still a type.

## How to find it

Go to {{file}}:{{line}} and decide which value you want:

- A new struct value: add the braces, `T{}` or `T{Field: v}`, or `&T{}` for a pointer.
- A conversion of a value to this type: `T(x)`.
- The zero value: `var v T`, then use `v`.
- A constant or variable whose name looks like the type: check the spelling.

## Example

```go
type Limits struct{ Retries int }

func Default() Limits {
	return Limits // Limits (type) is not an expression
}
```

Return a value of the type, written as a composite literal:

```go
func Default() Limits {
	return Limits{Retries: 3}
}
```

## Learn more
