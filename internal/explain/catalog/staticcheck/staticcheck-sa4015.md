---
id: staticcheck-sa4015
source: staticcheck
code: SA4015
match: ^calling (?P<func>\S+) on a converted integer is pointless$
title: {{func}} gets an integer, so there is nothing left to round
course: 03-latency
links:
- staticcheck: SA4015 | https://staticcheck.dev/docs/checks/#SA4015
- The Go spec: Integer operators | https://go.dev/ref/spec#Integer_operators
- Package math: Ceil | https://pkg.go.dev/math#Ceil
---
## What it means

The argument of `{{func}}` is an integer converted to a float. An integer has no fractional part, so `{{func}}` returns it unchanged. Usually the integer comes from a division: in `float64(a / b)` the integer division has already dropped the fraction before the conversion. The rounding that you wanted never happens.

## Why Go does this

When both operands are integers, `/` is integer division, and the Go spec says that it truncates towards zero: 7 / 2 is 3. A conversion afterwards cannot bring the fraction back. The compiler accepts the code, because each step is valid on its own.

## How to find it

Go to {{file}}:{{line}} and find the integer expression inside the conversion. Convert the operands before you divide, so that the division happens in floats: `float64(a) / float64(b)`. Then round with `{{func}}`. For positive integers you can also round up without floats: `(a + b - 1) / b`.

## Example

```go
pages := int(math.Ceil(float64(items / perPage))) // 7 / 2 is already 3
```

Divide as floats, then round up:

```go
pages := int(math.Ceil(float64(items) / float64(perPage)))
```

## Learn more

The course exercise below practises rounding with math.Ceil.
