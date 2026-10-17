---
id: runtime-nil-map
source: runtime
match: ^panic: assignment to entry in nil map$
title: {{func}} wrote to a map that was never made
links:
- The Go spec: Map types | https://go.dev/ref/spec#Map_types
- The Go spec: Index expressions | https://go.dev/ref/spec#Index_expressions
- Effective Go: Maps | https://go.dev/doc/effective_go#maps
- The Go blog: Go maps in action | https://go.dev/blog/maps#declaration-and-initialization
---
## What it means

At {{file}}:{{line}}, `{{func}}` stored a value in a map whose value is `nil`: the map variable exists, but no map was ever created for it. Reading from a nil map works and gives zero values, so the mistake shows only at the first write.

## Why Go does this

The zero value of a map type is `nil`, and a nil map has no table to store anything in. The spec says that assigning to an element of a nil map causes a run-time panic. Only `make(map[K]V)` or a map literal such as `map[K]V{}` creates a map. Reads, `len`, `range` and `delete` all work on a nil map; they treat it as empty.

## How to find it

Line {{line}} writes to a map: `m[k] = v`, `m[k]++` or `m[k] = append(m[k], x)`. Find where that map variable or field is declared, and look for the place that should create it:

- A struct field: a value made with `T{}` or `var t T` leaves the map nil. Does a constructor such as `NewT` create it, and does the caller use that constructor?
- A local `var m map[string]int`: add `make`, or use a literal.
- A map that a function returns: does it return `nil` on some path?

Create the map once, where the value that owns it is created. Creating it on first use (`if t.m == nil { t.m = make(...) }`) also works.

## Example

```go
type Tally struct {
	counts map[string]int
}

func (t *Tally) Add(word string) {
	t.counts[word]++ // counts is nil in a Tally{}
}
```

Create the map in a constructor:

```go
func NewTally() *Tally {
	return &Tally{counts: make(map[string]int)}
}
```

## Learn more
