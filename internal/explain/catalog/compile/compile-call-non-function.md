---
id: compile-call-non-function
source: compile
match: ^invalid operation: cannot call (?P<expr>.+) \((?P<what>.+)\): (?P<type>.+) is not a function$
title: {{expr}} is a value of type {{type}}, not a function, so you cannot call it
links:
- The Go spec: Calls | https://go.dev/ref/spec#Calls
- The Go spec: Declarations and scope | https://go.dev/ref/spec#Declarations_and_scope
- The Go spec: Predeclared identifiers | https://go.dev/ref/spec#Predeclared_identifiers
---
## What it means

At {{file}}:{{line}} you wrote `()` after `{{expr}}`, but `{{expr}}` is a value of type `{{type}}`. Only functions and methods can be called.

## Why Go does this

A call needs an expression of function type. Go looks up the name in the nearest enclosing scope first. A local variable therefore hides a package-level function or a built-in function with the same name, such as `len`, `min`, `max` or `new`, until the end of its block. After `max := 0`, the name `max` means your integer, not the built-in.

## How to find it

Go to {{file}}:{{line}} and find the declaration of `{{expr}}` that is closest to this line.

- A local variable with the name of a function or built-in: rename the variable after what it holds, for example `longest`, `total` or `limit`.
- A field that you meant as a method, such as `s.Name()` for a field `Name`: drop the `()`, or add the method if the API needs it.
- A conversion whose type name is hidden by a variable of the same name: rename the variable.

## Example

```go
func Longest(words []string) int {
	max := 0
	for _, w := range words {
		max = max(max, len(w)) // invalid operation: cannot call max (variable of type int): int is not a function
	}
	return max
}
```

Give the variable its own name, so `max` is the built-in again:

```go
	longest := 0
	for _, w := range words {
		longest = max(longest, len(w))
	}
	return longest
```

## Learn more
