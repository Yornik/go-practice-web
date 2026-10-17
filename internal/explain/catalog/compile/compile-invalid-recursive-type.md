---
id: compile-invalid-recursive-type
source: compile
match: ^invalid recursive type: (?P<type>\S+) refers to itself$
match: (?s)^invalid recursive type (?P<type>[^\s:]+)(?:\n.*)?$
title: {{type}} contains itself, so its size would be infinite
links:
- The Go spec: Struct types | https://go.dev/ref/spec#Struct_types
- The Go spec: Pointer types | https://go.dev/ref/spec#Pointer_types
- The Go spec: Type definitions | https://go.dev/ref/spec#Type_definitions
---
## What it means

The type `{{type}}` contains a field of its own type, directly or through other types. A value of `{{type}}` would then contain another `{{type}}`, which contains another one, without end. The indented lines under the message show the chain, such as `A refers to B` and `B refers to A`.

## Why Go does this

A struct holds its fields inline: a struct field is a whole value, not a reference. So the size of a struct is the sum of its fields, and a struct that contains itself would have no finite size. A pointer, a slice or a map has a fixed size whatever it points to, so it breaks the cycle.

## How to find it

Follow the chain from {{file}}:{{line}}, or from the `refers to` lines, until you find the field that leads back to `{{type}}`. Then decide how that field should refer to the other value:

- 1 optional value, such as a parent or a next element: a pointer, `*T`. A `nil` pointer means "none".
- Many values, such as children: a slice, `[]T` or `[]*T`.
- Values by name: a map, or store only an ID and look it up.

## Example

```go
type Item struct {
	Value int
	Next  Item // invalid recursive type: Item refers to itself
}
```

Point to the next item; `nil` ends the list:

```go
type Item struct {
	Value int
	Next  *Item
}
```

## Learn more
