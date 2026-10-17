---
id: compile-value-not-used
source: compile
match: ^(?P<expr>.+) \((?P<what>.+)\) is not used$
title: The value of {{expr}} is computed and then thrown away
links:
- The Go spec: Expression statements | https://go.dev/ref/spec#Expression_statements
- The Go spec: Appending to and copying slices | https://go.dev/ref/spec#Appending_and_copying_slices
- The Go blog: Arrays, slices (and strings): The mechanics of append | https://go.dev/blog/slices
---
## What it means

The statement at {{file}}:{{line}} computes the value `{{expr}}` and then does nothing with it. Go accepts only statements that do something, so it rejects a value that is dropped on the floor.

## Why Go does this

Of all expressions, only calls and channel receives can stand alone as a statement, and even then not the built-in functions that only compute a value, such as `append`, `len`, `min` and `max`. A computed value that nobody uses is almost always a bug. The classic one is `append`: it returns the new slice, which may live in a new backing array, and it does not change the slice variable you pass in.

## How to find it

Go to {{file}}:{{line}} and decide where the value should go:

- `append(xs, v)`: assign the result back, `xs = append(xs, v)`.
- A comparison or a calculation: did you mean an assignment, or an `if`?
- A plain variable after `switch` with the `{` on the next line: Go ends the line with a semicolon and reads `switch {{expr}}; {`, a switch on `true`. Put the `{` on the line of the `switch`.

## Example

```go
func Track(hits []string, path string) []string {
	append(hits, path) // append(hits, path) (value of type []string) is not used
	return hits
}
```

Keep the slice that `append` returns:

```go
func Track(hits []string, path string) []string {
	hits = append(hits, path)
	return hits
}
```

## Learn more
