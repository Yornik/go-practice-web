---
id: staticcheck-sa4012
source: staticcheck
code: SA4012
title: Nothing is equal to NaN, not even NaN itself
course: 28-latency
links:
- staticcheck: SA4012 | https://staticcheck.dev/docs/checks/#SA4012
- Package math: IsNaN | https://pkg.go.dev/math#IsNaN
- The Go spec: Comparison operators | https://go.dev/ref/spec#Comparison_operators
---
## What it means

You compare a value with `math.NaN()`. NaN, "not a number", is unequal to every value, itself included. So `x == math.NaN()` is always false and `x != math.NaN()` is always true, even when `x` is NaN. The check never finds the NaN it looks for, and the NaN flows on into your results.

## Why Go does this

The Go spec says that floating-point values compare as the IEEE 754 standard defines. In that standard, NaN is unordered: every comparison with it is false, except `!=`. A computation such as 0 divided by 0 in floats, or `math.Sqrt(-1)`, produces NaN.

## How to find it

Go to {{file}}:{{line}} and replace the comparison with `math.IsNaN(x)`. For infinities use `math.IsInf(x, 0)`. Then ask where a NaN can come from: an empty input that leads to 0 divided by 0, or a parsed `NaN` from text. Decide what your function should return for it, and test that case.

## Example

```go
if x != math.NaN() { // always true
	sum += x
}
```

Ask math whether the value is NaN:

```go
if !math.IsNaN(x) {
	sum += x
}
```

## Learn more

The course exercise below practises NaN and infinity in float computations.
