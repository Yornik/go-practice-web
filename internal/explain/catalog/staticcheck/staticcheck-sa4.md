---
id: staticcheck-sa4
source: staticcheck
code: SA4
title: This code has no effect ({{code}})
links:
- staticcheck: SA4, code that is not really doing anything | https://staticcheck.dev/docs/checks/#SA4
---
## What it means

The code at {{file}}:{{line}} runs, but it has no effect: it compares a value with itself, computes a result that nobody uses, or tests a condition that is always the same. The message says which. staticcheck groups these checks under SA4: code that is not really doing anything. This is check {{code}}.

## Why Go does this

The compiler checks that code is valid, not that it does something. staticcheck follows values through the function and reports code whose result can never matter. Such code is rarely only untidy. Usually it shows that something else is wrong: a variable name, a missing assignment, or a check in the wrong place.

## How to find it

Go to {{file}}:{{line}} and say aloud what the line should do. Then compare that with what it does. Look for a wrong variable name, as in the example below, a result that should be assigned, or a value that has the wrong type for the check. The staticcheck page explains every check with an example: search it for {{code}}.

## Example

```go
func Changed(prev, next int) bool {
	return prev != prev // always false
}
```

Compare the 2 values that you meant:

```go
func Changed(prev, next int) bool {
	return prev != next
}
```

## Learn more
