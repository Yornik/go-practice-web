---
id: staticcheck-sa4025
source: staticcheck
code: SA4025
match: ^the integer division '(?P<expr>[^']+)' results in zero$
title: The integer division {{expr}} is 0
course: 05-slo
links:
- staticcheck: SA4025 | https://staticcheck.dev/docs/checks/#SA4025
- The Go spec: Constant expressions | https://go.dev/ref/spec#Constant_expressions
- The Go spec: Integer operators | https://go.dev/ref/spec#Integer_operators
---
## What it means

Both operands of `{{expr}}` are integer constants, so Go divides them as integers and drops the fraction. `99 / 100` is 0, not 0.99. Every value that you compute from it is wrong, and a test with real numbers shows results such as 0 or 100% where you expect a fraction.

## Why Go does this

An expression of untyped integer constants gives an untyped integer constant, and integer division truncates. The Go spec shows it with `15 / 4`, which is 3, while `15 / 4.0` is 3.75. Using the result as a float later does not help: the fraction is gone by then. The compiler accepts it, because the expression is valid.

## How to find it

Go to {{file}}:{{line}} and decide which value you meant. Write 1 of the operands as a floating-point constant, such as `99.0 / 100`, or write the value directly, such as `0.99`. For variables, convert before you divide: `float64(good) / float64(total)`. Then compute 1 example by hand and compare it with your function.

## Example

```go
const target = 99 / 100 // 0
```

Make the division a float division:

```go
const target = 99.0 / 100 // 0.99
```

## Learn more

The course exercise below practises integer and float division in a formula.
