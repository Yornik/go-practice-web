---
id: staticcheck-sa5000
source: staticcheck
code: SA5000
title: A value is written to a nil map, which panics
course: 01-loglevels
links:
- staticcheck: SA5000 | https://staticcheck.dev/docs/checks/#SA5000
- The Go spec: Map types | https://go.dev/ref/spec#Map_types
- The Go blog: Go maps in action | https://go.dev/blog/maps
---
## What it means

The map is nil: you declared it, but never created it. Reading a nil map works and gives zero values, so the bug hides until the first write. Writing to it panics at run time with `assignment to entry in nil map`, and the test stops there.

## Why Go does this

A map value refers to a hash table, and the zero value of a map type refers to none. The Go spec says that a nil map is like an empty map, except that no elements may be added. `make` or a map literal creates the table. The compiler cannot know at every write whether a map was created, so the mistake shows up only when the code runs.

## How to find it

Go to {{file}}:{{line}} and follow the map back to its declaration. Create it with `make(map[K]V)` or a literal `map[K]V{}` before the first write:

- A local map: create it where you declare it.
- A map in a struct field: create it in the constructor, such as `New`, or check for nil in the method that writes first.
- A map in a struct inside another map: each inner map needs its own `make`.

## Example

```go
var counts map[string]int
for _, w := range words {
	counts[w]++ // panics: counts is nil
}
```

Create the map before you write to it:

```go
counts := make(map[string]int)
for _, w := range words {
	counts[w]++
}
```

## Learn more

The course exercise below practises maps that count and group values.
