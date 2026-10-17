---
id: compile-syntax-unexpected-token
source: compile
match: ^syntax error: unexpected (?P<found>.+?) at end of statement$
match: ^expected ';', found (?P<found>.+?)(?: \(and \d+ more errors?\))?$
title: The statement should end, but Go found {{found}}
links:
- The Go spec: Statements | https://go.dev/ref/spec#Statements
- The Go spec: For statements | https://go.dev/ref/spec#For_statements
- Effective Go: For | https://go.dev/doc/effective_go#for
---
## What it means

At {{file}}:{{line}}, Go read a complete statement and expected it to end there, with the end of the line or a `;`. Instead the line goes on with {{found}}. Often the first word of the line is not what Go thinks it is, so Go read only that word as a statement.

## Why Go does this

Go has a small set of statement keywords: `if`, `for`, `switch`, `select`, `return`, `go`, `defer` and a few more. A word that is not a keyword is a name, so `while n > 0` reads as an expression `while` followed by more text. Go has no `while`, `do`, `until`, `unless` or `foreach`; `for` covers all loops. 2 statements on 1 line need a `;` between them.

## How to find it

Go to {{file}}:{{line}} and read the line from the start:

- A keyword from another language: use `for cond { }` for a while loop, and `for { }` for an endless loop.
- 2 statements on 1 line: put each on its own line.
- A missing operator between 2 values, such as `a b` instead of `a + b`, or a missing `.` in a selector.

Fix this error first; the errors after it are often caused by it.

## Example

```go
func Countdown(n int) []int {
	var out []int
	while n > 0 { // syntax error: unexpected name n at end of statement
		out = append(out, n)
		n--
	}
	return out
}
```

Write the loop with `for`:

```go
	for n > 0 {
```

## Learn more
