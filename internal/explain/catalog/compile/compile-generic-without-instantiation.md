---
id: compile-generic-without-instantiation
source: compile
match: ^cannot use generic (?P<kind>type|function) (?P<name>.+?) without instantiation$
title: The generic {{kind}} {{name}} needs type arguments here
course: 17-flightrec
links:
- The Go spec: Instantiations | https://go.dev/ref/spec#Instantiations
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}} you used the generic {{kind}} `{{name}}` without saying what its type parameters are. A generic type or function is a template; before you can use it as a type or a value, it must be instantiated with type arguments.

## Why Go does this

For a generic type, Go never infers the type arguments: you always write them, as in `Ring[string]`. For a generic function, Go infers them from the arguments of a call or from the type of the variable it is assigned to. A bare `f := Largest` gives it no information, so `f` would have no type.

## How to find it

Look at how `{{name}}` is used at {{file}}:{{line}}:

- In a method receiver: name the type parameters, without constraints: `func (r *Ring[T]) Len() int`. The constraints come from the type declaration.
- In a variable, field, parameter or result type: give the type arguments, `*Ring[string]`, or use the type parameter of the enclosing generic function: `*Ring[T]`.
- A generic function as a value: instantiate it, `pick := Largest[int]`, or assign it to a variable with a function type.

## Example

```go
type Ring[T any] struct {
	items []T
}

func (r *Ring) Len() int { // cannot use generic type Ring[T any] without instantiation
	return len(r.items)
}
```

Declare the type parameters in the receiver:

```go
func (r *Ring[T]) Len() int {
	return len(r.items)
}
```

## Learn more

The course exercise below practises methods on a generic type.
