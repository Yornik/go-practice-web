---
id: compile-cannot-compare
source: compile
match: ^invalid operation: .+ \((?P<reason>(?:slice|map|func) can only be compared to nil|.+ cannot be compared|incomparable types in type set)\)$
match: ^invalid map key type (?P<reason>.+)$
title: These values have no ==, so Go cannot compare them or use them as map keys
course: 19-alertgroup
links:
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
- The Go spec: Map types | https://go.dev/ref/spec#Map_types
- Go FAQ: Why don't maps allow slices as keys? | https://go.dev/doc/faq#map_keys
---
## What it means

At {{file}}:{{line}} you used `==` or `!=` on values that Go cannot compare, or a type with such values as a map key. The message ends with the reason or the type: `{{reason}}`. Slices, maps and functions can only be compared with `nil`. A struct or an array that contains one of them cannot be compared at all.

## Why Go does this

Equality of a slice or a map has no single obvious meaning: the same backing array, or the same elements? Deep or shallow? Go leaves `==` undefined for them instead of guessing. A map needs `==` on its keys to find an entry, so the key type must be comparable. Strings, numbers, booleans, pointers, channels, interfaces and structs or arrays of those are comparable.

## How to find it

Decide what "equal" means for your values, then write that:

- 2 slices with the same elements: `slices.Equal(a, b)`.
- 2 maps with the same entries: `maps.Equal(a, b)`.
- A slice or a map as a map key: build a comparable key from it, such as a string with the elements in a fixed order, or an array if the length is fixed.
- A struct key with a slice field: keep only comparable fields in the key struct.

## Example

```go
func SameTags(a, b []string) bool {
	return a == b // invalid operation: a == b (slice can only be compared to nil)
}
```

Compare the elements:

```go
func SameTags(a, b []string) bool {
	return slices.Equal(a, b)
}
```

## Learn more

Exercise 19 builds canonical keys from maps.
