---
id: compile-method-on-non-local-type
source: compile
match: ^cannot define new methods on non-local type (?P<type>\S+)$
title: You cannot add methods to {{type}}, because this package does not declare it
links:
- The Go spec: Method declarations | https://go.dev/ref/spec#Method_declarations
- The Go spec: Type definitions | https://go.dev/ref/spec#Type_definitions
- Effective Go: Methods | https://go.dev/doc/effective_go#methods
---
## What it means

At {{file}}:{{line}} you declared a method with the receiver type `{{type}}`. A method can only be declared in the package that declares its receiver type. `{{type}}` comes from another package, or it is a predeclared type such as `int` or `string`.

## Why Go does this

The method set of a type decides which interfaces it implements. If any package could add methods to `time.Duration`, that set would depend on which packages happen to be imported, and 2 packages could add conflicting methods with the same name. So the spec requires the receiver base type to be declared in the same package as the method.

## How to find it

Choose 1 of these designs:

- A plain function that takes the value: `func Millis(d time.Duration) int64`. Often this is all you need.
- Your own type based on it: `type Timeout time.Duration`, with your methods on `Timeout`. A defined type does not inherit the methods of the type it is based on, so convert back with `time.Duration(t)` when you need them.
- A struct that embeds the type: it keeps the methods of the embedded type and gets yours.

## Example

```go
func (d time.Duration) Millis() int64 { // cannot define new methods on non-local type time.Duration
	return d.Milliseconds()
}
```

Write a function instead:

```go
func Millis(d time.Duration) int64 {
	return d.Milliseconds()
}
```

## Learn more
