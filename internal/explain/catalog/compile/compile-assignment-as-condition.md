---
id: compile-assignment-as-condition
source: compile
match: ^syntax error: cannot use (?:assignment )?.+ as value$
match: ^expected (?:boolean|boolean or range|switch) expression, found (?:assignment|simple statement) \(missing parentheses around composite literal\?\)$
title: The condition is an assignment, but Go needs a comparison
links:
- The Go spec: If statements | https://go.dev/ref/spec#If_statements
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
- Effective Go: If | https://go.dev/doc/effective_go#if
---
## What it means

At {{file}}:{{line}} the condition of an `if`, `for` or `switch` is an assignment such as `x = 1` or `y := x`, or another statement such as `n++`. A statement does not produce a value in Go, so it cannot be a condition. You almost always meant the comparison `==`.

## Why Go does this

In Go, `=` and `:=` are statements, not expressions. In C, `if (x = 1)` assigns and tests at the same time, a classic source of bugs. Go makes that line a syntax error. When you want to assign before the test, Go has its own form: a short statement before the condition, separated by a semicolon, as in `if err := f(); err != nil`.

## How to find it

Go to {{file}}:{{line}} and read the condition:

- A single `=` where you meant to compare: write `==`.
- An assignment that should happen first: move it into the init statement, `if v := f(); v > 0 {`, or onto its own line before the `if`.

The editor and the test run phrase this error differently. The editor says `expected boolean expression, found assignment` (or `switch expression`, or `simple statement`); the compiler says `cannot use assignment ... as value`.

## Example

```go
func Status(code int) string {
	if code = 200 { // syntax error: cannot use assignment code = 200 as value
		return "ok"
	}
	return "error"
}
```

Compare with `==`:

```go
	if code == 200 {
```

## Learn more
