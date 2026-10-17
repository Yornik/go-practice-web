---
id: compile-does-not-satisfy
source: compile
match: ^(?:in call to (?P<func>[^,]+), )?(?P<type>.+?) does not satisfy (?P<constraint>.+?)(?: \((?P<reason>.+)\))?$
title: {{type}} does not satisfy the constraint {{constraint}}
course: 09-cache
links:
- The Go spec: Satisfying a type constraint | https://go.dev/ref/spec#Satisfying_a_type_constraint
- The Go spec: Type constraints | https://go.dev/ref/spec#Type_constraints
- The Go blog: An Introduction To Generics | https://go.dev/blog/intro-generics
---
## What it means

At {{file}}:{{line}} a type argument, `{{type}}`, does not meet the constraint `{{constraint}}` of its type parameter. Go may have inferred that type from your arguments. The part in parentheses at the end, when present, says why.

## Why Go does this

A constraint lists what a generic function or type needs from its type parameter: methods, the operator `==` for `comparable`, or a set of types such as `~int | ~float64`. Go checks every type argument against it when it instantiates the code, so the generic body can rely on it.

## How to find it

Read the reason and decide which side is wrong:

- `comparable`: slices, maps and functions cannot be compared, so they cannot be map keys or `comparable` type arguments. Use a comparable key, such as a string built from the slice.
- `X missing in ...`: the type is not in the list of the constraint. Pass a type from the list, or widen the constraint if the generic code really works for this type.
- `missing method M`: the type lacks a method that the constraint requires.
- `method M has pointer receiver`: only the pointer type has `M`. Pass `[]*T` instead of `[]T`, or give `M` a value receiver.

## Example

```go
func Total[T Number](xs []T) T { ... } // type Number interface{ ~int | ~float64 }

s := Total([]string{"a", "b"}) // string does not satisfy Number (string missing in ~int | ~float64)
```

Pass values of a type that the constraint allows:

```go
s := Total([]float64{1.5, 2})
```

## Learn more

Exercise 09 practises `comparable` keys.
