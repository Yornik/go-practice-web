---
id: runtime-divide-by-zero
source: runtime
match: ^panic: runtime error: integer divide by zero$
title: {{func}} divided an integer by zero
links:
- The Go spec: Integer operators | https://go.dev/ref/spec#Integer_operators
- The Go spec: Floating-point operators | https://go.dev/ref/spec#Floating_point_operators
- Package math: IsNaN | https://pkg.go.dev/math#IsNaN
---
## What it means

At {{file}}:{{line}}, `{{func}}` computed `a / b` or `a % b` with integers, and `b` was 0 when the program ran.

## Why Go does this

There is no integer that is the result of a division by zero. The spec says that a divisor that is zero at run time causes a run-time panic; a constant zero divisor does not even compile.

Floating-point numbers behave differently. With Go's compiler, a `float64` divided by a zero variable gives `+Inf`, `-Inf` or, for 0 divided by 0, `NaN`, and nothing panics. The same mistake with `float64` therefore shows up later, as a strange value in a result or a comparison that is never true.

## How to find it

Find the divisor on line {{line}}. It is usually a count or a size: the length of an input, a number of buckets, a number of seconds, a total. Then ask:

1. Can this value be 0 for valid input? An empty slice is the most common case.
2. What does the spec of your function say the result is in that case? The README often names it.
3. Is it 0 because of an earlier integer division, such as `n / 1000` with `n` below 1000?

Handle the 0 case before the division, and return what the spec asks for.

## Example

```go
func Average(xs []int) int {
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum / len(xs) // panics for an empty xs
}
```

Decide what an empty input means, and check it first:

```go
	if len(xs) == 0 {
		return 0
	}
```

## Learn more
