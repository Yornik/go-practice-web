---
id: compile-break-outside-loop
source: compile
match: ^(?P<stmt>break|continue) is not in a loop(?:, switch, or select)?$
match: ^(?P<stmt>break|continue) not in for(?:, switch, or select)? statement$
title: {{stmt}} is not inside a loop of this function
links:
- The Go spec: Break statements | https://go.dev/ref/spec#Break_statements
- The Go spec: Continue statements | https://go.dev/ref/spec#Continue_statements
- The Go spec: Function literals | https://go.dev/ref/spec#Function_literals
---
## What it means

The `{{stmt}}` at {{file}}:{{line}} has no loop to act on. `break` needs an enclosing `for`, `switch` or `select`, and `continue` needs an enclosing `for`, in the same function. A function literal is a function of its own: a loop outside it does not count.

## Why Go does this

`break` and `continue` jump within 1 function. A closure may run later, in another goroutine, or after the loop has finished, so it cannot jump into the loop of the function that created it. An `if` is not a loop either: `break` inside an `if` that is not inside a loop has nothing to leave.

## How to find it

Go to {{file}}:{{line}} and walk outwards through the braces to the nearest `for`, `switch` or `select`:

- You meet a `func` first: the statement is in a closure. Make the closure return a value, such as a `bool`, and do the `break` or `continue` in the loop based on that value.
- There is no loop at all: use `return` to leave the function.
- A `goto` or a labeled statement will not help here: a label is visible only in its own function too.

## Example

```go
for i, x := range xs {
	func() {
		if x < 0 {
			found = i
			break // break is not in a loop, switch, or select
		}
	}()
}
```

Decide in the loop itself:

```go
for i, x := range xs {
	if x < 0 {
		found = i
		break
	}
}
```

## Learn more
