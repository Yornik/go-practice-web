---
id: vet-lostcancel-discarded
source: vet
analyzer: lostcancel
match: ^the cancel function returned by (?P<call>\S+) should be called, not discarded, to avoid a context leak$
title: The cancel function of {{call}} is thrown away
course: 21-rollout
links:
- Package context | https://pkg.go.dev/context
- The Go blog: Go Concurrency Patterns: Context | https://go.dev/blog/context
- The lostcancel check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel
---
## What it means

`{{call}}` returns a new context and a cancel function. You assigned the cancel function to `_`, so no code can ever call it. The new context and its timer stay alive until the timeout fires or the parent context ends. In a loop or a busy server, those leftovers pile up.

## Why Go does this

A derived context registers itself with its parent, and a timeout also starts a timer. The docs of package context say that calling the cancel function removes the child from its parent and stops its timers. Not calling it leaks the child until the parent ends. That is why the cancel function is a separate return value, and why `go vet` checks that you keep it.

## How to find it

Go to {{file}}:{{line}} and find the `_` in the assignment. Give the cancel function a name and call it when the work with the new context is done. In almost every case the right place is a `defer` on the next line. When the function returns the new context to its caller, return the cancel function too, so the caller can call it.

## Example

```go
ctx, _ = context.WithTimeout(ctx, 5*time.Second) // the cancel function is lost
```

Keep the cancel function and defer the call:

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
```

## Learn more

The course exercise below practises contexts with a deadline and who cancels them.
