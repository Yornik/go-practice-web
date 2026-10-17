---
id: compile-untyped-nil
source: compile
match: ^use of untyped nil in (?P<context>.+)$
title: nil alone has no type, so Go cannot tell which variable to make
links:
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
- The Go spec: The zero value | https://go.dev/ref/spec#The_zero_value
- The Go spec: Predeclared identifiers | https://go.dev/ref/spec#Predeclared_identifiers
---
## What it means

At {{file}}:{{line}} Go must find the type of a new variable from its initial value, as in `x := nil` or `var x = nil`. But `nil` fits many types: every pointer, slice, map, channel, function and interface type. It does not say which one you want, so Go cannot create the variable.

## Why Go does this

`:=` and `var x = value` take the type of the variable from the value. An untyped constant such as `0` or `"a"` has a default type (`int`, `string`), but `nil` has none. The spec says it directly: `nil` cannot initialize a variable with no explicit type.

## How to find it

Write the type of the variable yourself. You often need no value at all, because the zero value of these types is already `nil`:

- `var best *Server` for a pointer.
- `var err error` for an error.
- `var seen map[string]bool` for a map that you create later with `make`.

When you need a typed `nil` inside an expression, convert it: `(*Server)(nil)`.

## Example

```go
func Idlest(servers []*Server) *Server {
	best := nil // use of untyped nil in assignment
	for _, s := range servers {
		if best == nil || s.Load < best.Load {
			best = s
		}
	}
	return best
}
```

Declare the variable with its type; it starts as `nil`:

```go
	var best *Server
```

## Learn more
