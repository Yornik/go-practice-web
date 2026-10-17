---
id: staticcheck-sa4009
source: staticcheck
code: SA4009
match: ^argument (?P<name>\S+) is overwritten before first use$
title: The argument {{name}} is overwritten before the function reads it
links:
- staticcheck: SA4009 | https://staticcheck.dev/docs/checks/#SA4009
- The Go spec: Function declarations | https://go.dev/ref/spec#Function_declarations
---
## What it means

The function assigns a new value to its parameter `{{name}}` before any code reads the value that the caller passed. The argument of the caller has no effect at all: every call behaves the same. Often this is a test value left in the code, or a default that should apply only when the caller passed nothing.

## Why Go does this

A parameter is a local variable that starts with the value of the argument, so assigning to it is legal. The compiler only checks that the parameter is used somewhere. staticcheck follows the paths through the function and reports a parameter whose first value is never read.

## How to find it

The message points at the parameter `{{name}}`. Find the first assignment to it in the body: that is where the caller's value is lost. Then ask what you meant:

- A default for a missing value: assign only in that case, as in `if {{name}} <= 0 { ... }`.
- A separate local value: give it its own name, and keep `{{name}}` as it is.
- A leftover from a test or a debug session: delete it.

## Example

```go
func Limit(n int) int {
	n = 100 // the caller's n is lost
	if n > 50 {
```

Apply the default only when the caller gave no value:

```go
func Limit(n int) int {
	if n <= 0 {
		n = 100
	}
```

## Learn more
