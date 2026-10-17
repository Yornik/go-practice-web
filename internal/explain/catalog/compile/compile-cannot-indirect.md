---
id: compile-cannot-indirect
source: compile
match: ^invalid operation: cannot indirect (?P<expr>.+) \((?P<what>.+)\)$
title: {{expr}} is not a pointer, so * cannot follow it
links:
- The Go spec: Address operators | https://go.dev/ref/spec#Address_operators
- The Go spec: Pointer types | https://go.dev/ref/spec#Pointer_types
- Effective Go: Composite literals | https://go.dev/doc/effective_go#composite_literals
---
## What it means

At {{file}}:{{line}} you wrote `*` in front of `{{expr}}`, which is a {{what}}, not a pointer. In an expression, `*p` follows the pointer `p` to the value it points to. Only a pointer can be followed.

## Why Go does this

Go uses 2 operators for pointers. `&x` takes the address of `x` and gives a pointer. `*p` follows a pointer and gives the value. In a type, `*T` means "pointer to `T`", but in an expression `*` always follows a pointer. So `*T{...}` does not create a pointer: Go reads it as "follow the struct value `T{...}`", which is not a pointer.

## How to find it

Go to {{file}}:{{line}} and decide which of the 2 you meant:

- You want a pointer to a new value: write `&T{...}`, or `new(T)` for the zero value.
- You want the value behind a pointer: check that the operand really is a pointer. Maybe it is already a value and needs no `*`.

When you are not sure what a variable is, find its declaration: it shows the type.

## Example

```go
func Origin() *Point {
	return *Point{X: 0, Y: 0} // invalid operation: cannot indirect Point{…} (value of struct type Point)
}
```

Take the address of the new value with `&`:

```go
	return &Point{X: 0, Y: 0}
```

## Learn more
