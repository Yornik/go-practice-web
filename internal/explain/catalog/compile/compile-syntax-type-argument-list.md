---
id: compile-syntax-type-argument-list
source: compile
match: ^syntax error: unexpected name \S+, expected \]$
match: ^missing ',' in type argument list(?: \(and \d+ more errors?\))?$
title: A receiver lists type parameter names only, without constraints
course: 09-cache
links:
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
- The Go spec: Instantiations | https://go.dev/ref/spec#Instantiations
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}}, inside the square brackets after a generic type, Go found 2 words without a comma between them. Most often this is a method receiver with a constraint, such as `func (r *Ring[T any])`. It can also be a list of type arguments that misses a comma, such as `Two[int string]`.

## Why Go does this

The receiver of a method on a generic type looks like an instantiation: `*Ring[T]` gives a name to each type parameter of `Ring`, so the method can use it. The constraints are already part of the type declaration, `type Ring[T any] struct`, and the receiver takes them from there. Go therefore reads the brackets of a receiver as a list of names, separated by commas, and `T any` is 2 names without a comma.

## How to find it

Go to {{file}}:{{line}} and look at the square brackets:

- In a method receiver: keep only the names, `func (r *Ring[T]) Len() int`. With several type parameters, list them all, `[K, V]`, in the order of the type declaration.
- In a type such as a variable or field type: separate the type arguments with commas, `Two[int, string]`.

The constraints go only on the `type` declaration and on generic functions.

## Example

```go
type Ring[T any] struct {
	items []T
}

func (r *Ring[T any]) Len() int { // syntax error: unexpected name any, expected ]
	return len(r.items)
}
```

Name the type parameter in the receiver, without its constraint:

```go
func (r *Ring[T]) Len() int {
	return len(r.items)
}
```

## Learn more

Exercise 09 practises methods on a generic type.
