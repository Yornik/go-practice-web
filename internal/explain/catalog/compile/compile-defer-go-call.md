---
id: compile-defer-go-call
source: compile
match: ^expression in (?P<stmt>defer|go) must be function call(?: \(and \d+ more errors?\))?$
title: {{stmt}} needs a function call, with parentheses
links:
- The Go spec: Defer statements | https://go.dev/ref/spec#Defer_statements
- The Go spec: Go statements | https://go.dev/ref/spec#Go_statements
- The Go blog: Defer, Panic, and Recover | https://go.dev/blog/defer-panic-and-recover
---
## What it means

The `{{stmt}}` statement at {{file}}:{{line}} names a function but does not call it. `mu.Unlock` is a function value; `mu.Unlock()` is a call. `defer` and `go` need the call.

## Why Go does this

`defer` and `go` take a call expression. When the statement runs, Go evaluates the function value and its arguments right away. `defer` then runs the call when the surrounding function returns, and `go` runs it in a new goroutine. Without the parentheses there is no call to run later, so Go rejects the statement.

## How to find it

Go to {{file}}:{{line}} and add the parentheses with the arguments.

- To run several statements, wrap them in a function literal and call it: `defer func() { ... }()`. The final `()` is easy to forget.
- Remember that the arguments are evaluated when the `defer` or `go` statement runs, not later. `defer fmt.Println(n)` prints the value that `n` had at that moment.
- In `go func() { ... }()` inside a loop, pass the values the goroutine needs as arguments or capture per-iteration variables. Since Go 1.22, each loop iteration has its own loop variables.

## Example

```go
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock // expression in defer must be function call
	c.n++
}
```

Call the method, so the unlock runs when `Inc` returns:

```go
	defer c.mu.Unlock()
```

## Learn more
