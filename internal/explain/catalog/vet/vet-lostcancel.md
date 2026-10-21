---
id: vet-lostcancel
source: vet
analyzer: lostcancel
title: A cancel function is not called on every path
course: 21-rollout
links:
- Package context | https://pkg.go.dev/context
- The Go spec: Defer statements | https://go.dev/ref/spec#Defer_statements
- The lostcancel check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel
---
## What it means

The function creates a context with a cancel function, for example with `context.WithTimeout`, but at least 1 path through the function returns without calling it. On that path the context and its timer stay alive until the timeout fires or the parent ends. `go vet` reports 2 lines: the line that creates the cancel function, and a `return` that skips the call.

## Why Go does this

A derived context registers itself with its parent, and a timeout also starts a timer. The docs of package context say that the cancel function removes the child from its parent and stops its timers, and that `go vet` checks that every path calls it. Calling it twice is harmless: a second call does nothing.

## How to find it

Go to {{file}}:{{line}}. Follow every path from there to a `return`: each early return, each case of a `switch` or `select`, each error branch. The second message names a return that skips the call. You rarely need to add a call to each path. Put `defer cancel()` right after the line that creates the context, and every path is covered.

## Example

```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
select {
case <-ready:
	return nil // cancel is never called on this path
case <-ctx.Done():
	cancel()
	return ctx.Err()
}
```

Defer the call once, right after you create the context:

```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
defer cancel()
```

## Learn more
