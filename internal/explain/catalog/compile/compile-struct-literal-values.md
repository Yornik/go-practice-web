---
id: compile-struct-literal-values
source: compile
match: ^(?P<problem>too (?:few|many) values) in struct literal of type .+$
match: ^(?P<problem>mixture of field:value and value elements) in struct literal$
title: The values of this struct literal do not line up with its fields
links:
- The Go spec: Composite literals | https://go.dev/ref/spec#Composite_literals
- Effective Go: Composite literals | https://go.dev/doc/effective_go#composite_literals
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
---
## What it means

The struct literal at {{file}}:{{line}} has {{problem}}. A struct literal comes in 2 forms. Without keys, `T{1, 2}`, it must list a value for every field, in the order of the declaration. With keys, `T{X: 1}`, it may list any fields in any order. You cannot mix the 2 forms.

## Why Go does this

The form without keys depends on the exact list and order of the fields, so Go checks that the list is complete. The keyed form names each field, so missing fields get their zero value, and adding a field to the struct later does not break the literal. Mixing would make it unclear which value goes to which field.

## How to find it

Go to {{file}}:{{line}}, open the declaration of the struct, and count its fields.

- Prefer keys: `T{Name: "a", Port: 80}`. Fields you leave out get their zero value.
- Without keys, give exactly 1 value per field, in declaration order.
- In a mixture, add the field name to every value.

## Example

```go
type Point struct{ X, Y int }

func Start() Point {
	return Point{1} // too few values in struct literal of type Point
}
```

Name the fields; `Y` gets its zero value:

```go
	return Point{X: 1}
```

## Learn more
