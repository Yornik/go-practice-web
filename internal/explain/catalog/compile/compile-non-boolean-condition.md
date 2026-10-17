---
id: compile-non-boolean-condition
source: compile
match: ^non-boolean condition in (?P<stmt>if|for) statement$
title: The condition of this {{stmt}} statement is not a bool
links:
- The Go spec: If statements | https://go.dev/ref/spec#If_statements
- The Go spec: For statements with single condition | https://go.dev/ref/spec#For_condition
- The Go spec: Boolean types | https://go.dev/ref/spec#Boolean_types
---
## What it means

The condition of the `{{stmt}}` at {{file}}:{{line}} has another type than `bool`. Go does not treat `0`, `""`, `nil` or an empty slice as false, and it does not treat other values as true.

## Why Go does this

The condition of an `if` or a `for` must be a boolean expression. Languages that turn any value into true or false invite mistakes such as testing a count when you meant to test an error. In Go you write the comparison, and the reader sees what is tested.

## How to find it

Go to {{file}}:{{line}} and write the comparison you mean:

- A number: `n > 0` or `n != 0`.
- A string: `s != ""`.
- A slice or a map: `len(xs) > 0`.
- A pointer, an error or another interface: `err != nil`.
- A method that returns a `bool`: call it. `if s.Empty` names the method; `if s.Empty()` calls it.

## Example

```go
func Status(failures int) string {
	if failures { // non-boolean condition in if statement
		return "failing"
	}
	return "ok"
}
```

Compare the number:

```go
	if failures > 0 {
```

## Learn more
