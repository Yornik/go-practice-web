---
id: compile-type-switch-on-type-parameter
source: compile
match: ^(?:invalid operation: )?cannot use type (?P<kind>switch|assertion) on type parameter value (?P<expr>.+?) \((?P<what>.+)\)$
title: A type {{kind}} does not work on {{expr}}, a value of a type parameter
links:
- The Go spec: Type switches | https://go.dev/ref/spec#Type_switches
- The Go spec: Type assertions | https://go.dev/ref/spec#Type_assertions
- The Go blog: When To Use Generics | https://go.dev/blog/when-generics
---
## What it means

At {{file}}:{{line}} you wrote a type {{kind}} on `{{expr}}` ({{what}}). Type switches and type assertions work only on interface values. A value of a type parameter has 1 fixed type for each instantiation, not a dynamic type that can vary.

## Why Go does this

A type assertion asks which type an interface value holds at run time. A type parameter is not an interface value: in `Format[int]`, `v` is an `int` and nothing else. Code that branches on the type of `T` is also a sign that the function may not need to be generic: the point of a type parameter is code that works the same for every type.

## How to find it

Decide what the code should do for different types:

- If each type needs different code, take an interface value instead of a type parameter: `func Format(v any) string`, and use the type switch on it.
- If you need it only in 1 place, convert the value to an interface first: `switch x := any(v).(type)`. This compiles and works, but think about whether the function should be generic at all.
- If the behaviour differs only by a method, put the method in the constraint and call it.

## Example

```go
func Format[T any](v T) string {
	switch x := v.(type) { // cannot use type switch on type parameter value v (variable of type T constrained by any)
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	}
	return "?"
}
```

Take an interface value when the code depends on the dynamic type:

```go
func Format(v any) string {
	switch x := v.(type) {
```

## Learn more
