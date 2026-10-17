---
id: staticcheck-s1021
source: staticcheck
code: S1021
title: Declare the variable with its first value
links:
- staticcheck: S1021 | https://staticcheck.dev/docs/checks/#S1021
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
---
## What it means

You declare a variable with `var` and give it its value on the next line. 1 statement can do both. Between the 2 lines the variable holds a zero value that nobody uses, and the reader must look at 2 places to see what it holds.

## Why Go does this

A variable declaration can carry its initial value, as in `var msg = "hello"`, and inside a function `msg := "hello"` is the usual short form. A separate declaration is useful only when the value comes later, for example from different branches of an `if`. staticcheck suggests merging when the next line is the only assignment.

## How to find it

Go to {{file}}:{{line}} and merge the 2 lines: `x := value`, or `var x T = value` when you need a type other than the default type of the value. Keep the separate `var` when different paths assign different values, or when the variable must exist before a block such as a `for` loop.

## Example

```go
var msg string
msg = "hello, " + name
```

Declare the variable with its value:

```go
msg := "hello, " + name
```

## Learn more
