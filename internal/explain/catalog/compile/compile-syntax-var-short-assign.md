---
id: compile-syntax-var-short-assign
source: compile
match: ^syntax error: unexpected :=, expected =$
match: ^expected type, found ':='(?: \(and \d+ more errors?\))?$
title: var and := are 2 ways to declare; use 1 of them
links:
- The Go spec: Variable declarations | https://go.dev/ref/spec#Variable_declarations
- The Go spec: Short variable declarations | https://go.dev/ref/spec#Short_variable_declarations
- Effective Go: Variables | https://go.dev/doc/effective_go#variables
---
## What it means

At {{file}}:{{line}} you wrote `var x := value`. That mixes the 2 ways Go has to declare a variable. After `var` and the name, Go expects a type, `=` or the end of the line, not `:=`.

## Why Go does this

Go has a long form and a short form. `var x int` or `var x = 1` is the long form: it works everywhere, can name the type, and can leave out the value. `x := 1` is the short form: it works only inside functions and always takes the type from the value. The 2 forms have their own syntax, so they cannot be combined.

## How to find it

Go to {{file}}:{{line}} and pick 1 form:

- Inside a function, with a value: `sum := 0`.
- With an explicit type or at the package level: `var sum int` or `var sum = 0`.
- When the zero value is what you want, `var sum int` says so best.

## Example

```go
func Total(xs []int) int {
	var sum := 0 // syntax error: unexpected :=, expected =
	for _, x := range xs {
		sum += x
	}
	return sum
}
```

Use the short form alone:

```go
	sum := 0
```

## Learn more
