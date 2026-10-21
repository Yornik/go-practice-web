---
id: runtime-reflect-not-assignable
source: runtime
match: ^panic: reflect\.Set: value of type (?P<from>.+) is not assignable to type (?P<to>.+)$
title: reflect: a {{from}} cannot be stored in a {{to}}
course: 31-envconfig
links:
- The Go spec: Assignability | https://go.dev/ref/spec#Assignability
- Package reflect: Value.Set | https://pkg.go.dev/reflect#Value.Set
- Package reflect: Value.Convert | https://pkg.go.dev/reflect#Value.Convert
---
## What it means

At {{file}}:{{line}}, `{{func}}` called `Set` to store a value of type `{{from}}` in a place of type `{{to}}`. Go would reject the same assignment with `=` at compile time; through reflection the check happens at run time, and it panics.

## Why Go does this

`reflect.Value.Set` follows the assignability rules of the spec: a value can be stored only where its type fits, such as the identical type or an interface it implements. Reflection never converts or parses for you. A string is not an int, even when it holds digits, and an `int` is not an `int64`.

## How to find it

Print both types right before the line: `f.Type()` for the destination and `x.Type()` for the new value. Then pick the step that is missing:

- The types are both numbers, or a named type and its underlying type: convert first with `x.Convert(f.Type())`. `CanConvert` tells you whether that works.
- The new value is text: parse it into the destination's kind first, then use the setter for that kind, such as `SetInt`.
- You are not sure: check `x.Type().AssignableTo(f.Type())` before `Set`, and report an error instead of panicking.

## Example

```go
n := 3 // an int
f := reflect.ValueOf(&opts).Elem().FieldByName("Retries") // an int64 field
f.Set(reflect.ValueOf(n)) // int is not assignable to int64
```

Convert to the field's type first:

```go
f.Set(reflect.ValueOf(n).Convert(f.Type()))
```

## Learn more
