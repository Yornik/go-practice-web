---
id: compile-cannot-range
source: compile
match: ^cannot range over (?P<expr>.+?) \((?P<what>[^:\n]+)\)$
title: A for range loop cannot iterate over {{expr}}
links:
- The Go spec: For statements with range clause | https://go.dev/ref/spec#For_range
- The Go blog: Range Over Function Types | https://go.dev/blog/range-functions
- Package maps | https://pkg.go.dev/maps
---
## What it means

At {{file}}:{{line}} you wrote `for ... range {{expr}}`, but `{{expr}}` is a {{what}}, and Go does not know how to iterate over that type. Go never looks inside a struct to find something to loop over.

## Why Go does this

`range` works on a fixed list of kinds: an array, a pointer to an array, a slice, a string, a map, a channel you can receive from, an integer, and an iterator function such as `iter.Seq`. A struct, a pointer to a slice or a pointer to a map is none of these. Your own type can support `range` by offering a method that returns an iterator.

## How to find it

Look at the type in the message:

- A struct: range over the field that holds the data, `s.items`, or add a method that returns an iterator, such as `func (s Set) All() iter.Seq[string]`.
- A pointer to a slice or a map: follow the pointer, `range *xs`. Ask why you have a pointer to a slice; a slice is already a small header that refers to its elements.
- A number of a type that is not an integer, such as a `float64`: convert it, or loop with a classic `for i := 0; i < n; i++`.
- The sorted keys of a map: `slices.Sorted(maps.Keys(m))`.

## Example

```go
type Set struct {
	items map[string]bool
}

func Count(s Set) int {
	n := 0
	for range s { // cannot range over s (variable of struct type Set)
		n++
	}
	return n
}
```

Range over the field that holds the elements:

```go
	for range s.items {
		n++
	}
```

## Learn more
