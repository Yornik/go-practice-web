---
id: compile-constant-truncated
source: compile
match: ^(?P<value>.+?) \(untyped float constant(?: [^)]+)?\) truncated to (?P<type>\S+)$
match: ^cannot use (?P<value>.+?) \(untyped float constant(?: [^)]+)?\) as (?P<type>\S+) value in .+ \(truncated\)$
title: The constant {{value}} has a fraction, but {{type}} holds only whole numbers
links:
- The Go spec: Constants | https://go.dev/ref/spec#Constants
- The Go blog: Constants | https://go.dev/blog/constants
- Package time: Duration | https://pkg.go.dev/time#Duration
---
## What it means

At {{file}}:{{line}}, the constant `{{value}}` meets an integer type, `{{type}}`, so Go would have to drop the fraction. Go refuses to do that silently. For `time.Duration` the message names `int64`, the underlying type of a duration.

## Why Go does this

An untyped constant takes the type of the other operand or of the variable it goes into, and its value must be exactly representable in that type. `1.5` is not a whole number, so it cannot become an `int` or a `time.Duration`. Rounding would hide a mistake, such as a ratio that you meant to keep as a `float64`.

## How to find it

Look at what the expression should produce:

- A duration with a fraction of a unit: use a smaller unit, `1500 * time.Millisecond`, or compute in `float64` and convert at the end: `time.Duration(1.5 * float64(time.Second))`.
- A calculation such as `n * 0.5` with an `int` variable `n`: the result is an `int`. If you need the fraction, convert first: `float64(n) * 0.5`.
- A ratio or a rate: does the variable really need to be an `int`? Declare it as `float64`.

## Example

```go
func Grace() time.Duration {
	return 1.5 * time.Second // 1.5 (untyped float constant) truncated to int64
}
```

Use a unit that makes the value whole:

```go
	return 1500 * time.Millisecond
```

## Learn more
