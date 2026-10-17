---
id: compile-mismatched-types
source: compile
match: ^invalid operation: (?P<expr>.+) \(mismatched types (?P<left>(?:[^u]|u[^n]|un[^t]).*?) and (?P<right>(?:[^u]|u[^n]|un[^t]).*)\)$
match: ^invalid case (?P<expr>.+) in switch on .+ \(mismatched types (?P<left>(?:[^u]|u[^n]|un[^t]).*?) and (?P<right>(?:[^u]|u[^n]|un[^t]).*)\)$
title: {{expr}} mixes 2 types: {{left}} and {{right}}
links:
- The Go spec: Operators | https://go.dev/ref/spec#Operators
- The Go spec: Conversions | https://go.dev/ref/spec#Conversions
- Package time: Duration | https://pkg.go.dev/time#Duration
---
## What it means

The expression `{{expr}}` at {{file}}:{{line}} combines a value of type `{{left}}` with a value of type `{{right}}`. Arithmetic needs 2 operands of the same type, a comparison needs 1 operand that fits the type of the other, and Go does not convert either side for you. For a `switch`, each case value must have the type of the `switch` expression.

## Why Go does this

Except for shifts and untyped constants, both operands of an arithmetic operator must have identical types. In a comparison, 1 operand must be assignable to the type of the other, as an `*os.File` is to an `io.Reader`. Go has no implicit numeric conversions, because they hide lost precision and overflow. A named type is its own type, too: `time.Duration` is not `int64`, although it is built on it. That is why `time.Second * 2` compiles, because `2` is an untyped constant, while `time.Second * n` with an `int` variable `n` does not.

## How to find it

Decide which type the result must have, then convert the other operand:

- `time.Duration` and `int`: `time.Duration(n) * time.Second`.
- `float64` and `int`: `sum / float64(n)`. Convert before you divide: `float64(a / b)` converts a result that integer division already cut off.
- `int` and `int64`: convert to the type the result needs, such as `int64(n)`.

## Example

```go
func Timeout(retries int) time.Duration {
	return time.Second * retries // invalid operation: time.Second * retries (mismatched types time.Duration and int)
}
```

Convert the count to a `time.Duration`, then multiply:

```go
	return time.Duration(retries) * time.Second
```

## Learn more
