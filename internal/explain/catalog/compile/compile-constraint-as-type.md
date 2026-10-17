---
id: compile-constraint-as-type
source: compile
match: ^cannot use type (?P<name>\S+) outside a type constraint: (?P<reason>.+)$
title: {{name}} is a constraint, so it can only be used in a type parameter list
course: 09-cache
links:
- The Go spec: General interfaces | https://go.dev/ref/spec#General_interfaces
- The Go spec: Type constraints | https://go.dev/ref/spec#Type_constraints
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}} you used `{{name}}` as the type of a variable, a field, a parameter or a result. But `{{name}}` is an interface that only describes type parameters: {{reason}}. `comparable`, and any interface with a type list such as `~int | ~float64`, can only appear in square brackets, as the constraint of a type parameter.

## Why Go does this

An ordinary interface describes values: any value whose type has the methods. A constraint can also say "one of these types" or "supports `==`". A variable of such a type would need a dynamic type that is checked at run time, and Go does not support that. So these interfaces exist only at compile time, in type parameter lists.

## How to find it

Decide what the code should be generic over:

- Make the function or type generic, and use the constraint for its type parameter: `type entry[K comparable, V any] struct{ key K; value V }`. Then use `K` as the field type.
- If the value really can be of any type at run time, use `any`. A map key of type `any` compiles, but `==` on a slice inside it panics at run time.
- Inside a generic type, refer to the type parameter, `K`, never to its constraint.

## Example

```go
type entry struct {
	key   comparable // cannot use type comparable outside a type constraint: interface is (or embeds) comparable
	value any
}
```

Make the type generic and give the field the type parameter:

```go
type entry[K comparable, V any] struct {
	key   K
	value V
}
```

## Learn more

Exercise 09 practises comparable keys in a generic type.
