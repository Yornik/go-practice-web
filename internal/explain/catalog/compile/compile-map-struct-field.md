---
id: compile-map-struct-field
source: compile
match: ^cannot assign to struct field (?P<expr>.+) in map$
match: ^invalid operation: cannot take address of (?P<expr>.+) \(map index expression of (?:\w+ )?type .+\)$
title: A struct inside a map cannot be changed in place
course: 27-accesslog
links:
- The Go spec: Index expressions | https://go.dev/ref/spec#Index_expressions
- The Go spec: Address operators | https://go.dev/ref/spec#Address_operators
- The Go blog: Go maps in action | https://go.dev/blog/maps
---
## What it means

At {{file}}:{{line}} you tried to change a field of a struct that is stored in a map, `{{expr}}`, or to take the address of a map element. A map gives you a copy of the element when you read it. You can replace the whole element, but you cannot change a part of it in place, and you cannot point into it.

## Why Go does this

Map elements are not addressable. A map moves its elements in memory when it grows, so a pointer to an element would soon point to the wrong place. Assigning to a field needs exactly such an address. Replacing the element with `m[k] = v` is fine, because the map itself does the store.

## How to find it

Decide where the struct should live, then pick 1 of these:

- Read, change, write back: `v := m[k]`, change `v`, then `m[k] = v`. Simple, and right for small structs.
- Store pointers in the map: `map[string]*T`. Then `m[k].Field++` changes the struct the map points to. Create the struct when the key is new, because a missing key gives `nil`.

With pointers, every holder of the pointer sees the change. With values, each read is a separate copy.

## Example

```go
type Stock struct {
	Qty      int
	Reserved int
}

func Reserve(stock map[string]Stock, sku string, n int) {
	stock[sku].Qty -= n // cannot assign to struct field stock[sku].Qty in map
	stock[sku].Reserved += n
}
```

Copy the element out, change the copy, and store it back:

```go
	s := stock[sku]
	s.Qty -= n
	s.Reserved += n
	stock[sku] = s
```

## Learn more

The course exercise below practises maps of structs.
