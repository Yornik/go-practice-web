---
id: compile-cannot-infer
source: compile
match: ^in call to (?P<func>.+?), cannot infer (?P<param>\S+)(?: \(declared at .+\))?$
match: ^in call to (?P<func>.+?), .*\(cannot infer (?P<param>[^)]+)\)$
title: Go cannot work out the type parameter {{param}} of {{func}}
course: 17-flightrec
links:
- The Go spec: Instantiations | https://go.dev/ref/spec#Instantiations
- The Go spec: Type inference | https://go.dev/ref/spec#Type_inference
- The Go blog: Everything You Always Wanted to Know About Type Inference | https://go.dev/blog/type-inference
---
## What it means

`{{func}}` is generic, and the call at {{file}}:{{line}} gives Go nothing to find the type argument for `{{param}}` from. In a call, Go infers type arguments only from the types of the ordinary arguments. Here no argument tells it what `{{param}}` is.

## Why Go does this

Type inference only uses information that is in the call. A type parameter that appears only in the result, such as `func Zero[T any]() T`, cannot be inferred from `x := Zero()`, because `x` gets its type from the call, not the other way round. An untyped `nil` argument has no type to infer from either. For a generic type, you always write the type arguments.

## How to find it

Go to {{file}}:{{line}} and give Go the missing type:

- Write the type argument: `Zero[int]()`. You may leave out type arguments only from the right: `F[int](x)` sets the first and infers the rest.
- Pass an argument that has a type, instead of `nil` or a bare constant: `Count(map[string]int(nil))`, or declare a typed variable first.
- A declared type on the result does not help: `var n int = Zero()` fails the same way. Go infers from a target type only when you use the function itself as a value, as in `var f func() int = Zero`.

## Example

```go
func Zero[T any]() T {
	var z T
	return z
}

n := Zero() // in call to Zero, cannot infer T
```

Write the type argument:

```go
n := Zero[int]()
```

## Learn more

The course exercise below practises generic types and functions.
