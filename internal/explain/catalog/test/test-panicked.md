---
id: test-panicked
source: test
match: (?s)^(?:[^\n]*?\s)?panicked(?: [a-z ]+| ?,[a-z ]+)?(?::| with) (?P<value>"?(?:[^T"\n]|T[^O\n])(?:[^;\n]|;[^ \n])*)(?:\n.*)?$
title: Your code panicked: {{value}}
links:
- The Go blog: Defer, Panic, and Recover | https://go.dev/blog/defer-panic-and-recover
- The Go spec: Handling panics | https://go.dev/ref/spec#Handling_panics
- Package runtime/debug: Stack | https://pkg.go.dev/runtime/debug#Stack
---
## What it means

Your code panicked while the test ran it, with the value `{{value}}`. A helper in the test recovered the panic and turned it into this failure at {{file}}:{{line}}, so the other tests still run. In a stub file, that is the line of the panic. A file that is not yours, such as `panic.go` or `chan.go`, is in the Go runtime, which panicked for an operation in your code.

## Why Go does this

`recover` returns the value that was passed to `panic`, but not the stack trace. So the message tells you what went wrong, not where. The value names the kind of mistake:

- `runtime error: index out of range` or `slice bounds out of range`: an index or a slice expression.
- `runtime error: invalid memory address or nil pointer dereference`: a field or a call through a nil pointer, interface or function.
- `assignment to entry in nil map`: a write to a map that was never made.
- `send on closed channel`, `close of closed channel`: the channel rules.
- `reflect: ...`: a reflect call that does not fit the value.

## How to find it

Search the call that the message names, and what it calls, for the operation that the value names. If the test printed a stack under the message, read it: the first frame in the exercise package is the line. To get a stack yourself, put this at the top of the function for a moment:

```go
defer func() {
	if v := recover(); v != nil {
		fmt.Printf("%v\n%s", v, debug.Stack())
		panic(v)
	}
}()
```

## Example

```text
x_test.go:10: Add("a") panicked: assignment to entry in nil map
```

The value names a map write. In `Add`, the map was never made:

```go
func (t *Tally) Add(w string) {
	t.counts[w]++ // counts is nil
}
```

## Learn more
