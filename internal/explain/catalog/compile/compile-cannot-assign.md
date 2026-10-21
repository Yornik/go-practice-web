---
id: compile-cannot-assign
source: compile
match: ^cannot assign to (?P<expr>.+) \(neither addressable nor a map index expression\)$
title: {{expr}} cannot be assigned to
course: 26-logfmt
links:
- The Go spec: Assignment statements | https://go.dev/ref/spec#Assignment_statements
- The Go spec: String types | https://go.dev/ref/spec#String_types
- The Go blog: Strings, bytes, runes and characters in Go | https://go.dev/blog/strings
---
## What it means

At {{file}}:{{line}} the left side of the assignment, `{{expr}}`, is not a place that can hold a new value. Go can only assign to a variable, a pointer target, a slice or array element, a field of such a value, or a map element. Common causes: a byte of a string, a constant, or the result of a function call.

## Why Go does this

Strings in Go are immutable: `s[i]` reads a byte, but no code can change the bytes of a string. That lets Go share a string, and substrings of it, between goroutines without copying. Constants are values computed at compile time and have no storage. A function result is a temporary value that disappears after the statement.

## How to find it

Look at what `{{expr}}` is:

- A byte of a string: build a new string instead. Convert to `[]byte` or `[]rune`, change that, and convert back. Or use `strings.Builder`, or functions such as `strings.ToUpper` and `strings.Replace`.
- A constant: declare a variable if the value must change.
- A field of a function result, such as `f().Name = x`: store the result in a variable first.

## Example

```go
func Capitalize(s string) string {
	s[0] = s[0] - 'a' + 'A' // cannot assign to s[0] (neither addressable nor a map index expression)
	return s
}
```

Change a copy of the bytes, then build a new string:

```go
	b := []byte(s)
	b[0] = b[0] - 'a' + 'A'
	return string(b)
```

## Learn more

The course exercise below treats a string as bytes in a hand-written scanner.
