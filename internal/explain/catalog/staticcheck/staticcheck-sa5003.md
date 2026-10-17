---
id: staticcheck-sa5003
source: staticcheck
code: SA5003
title: The deferred call in this endless loop never runs
links:
- staticcheck: SA5003 | https://staticcheck.dev/docs/checks/#SA5003
- The Go spec: Defer statements | https://go.dev/ref/spec#Defer_statements
- Effective Go: Defer | https://go.dev/doc/effective_go#defer
---
## What it means

The loop has no way out, so the function never returns. A deferred call runs only when the surrounding function returns, not at the end of a loop iteration. Every iteration adds 1 more deferred call that never runs. A deferred `Unlock` never unlocks, so the second iteration blocks forever on `Lock`. A deferred `Close` never closes, so files or connections pile up.

## Why Go does this

The Go spec says that deferred functions run immediately before the surrounding function returns. A `defer` belongs to the function, not to the block or the loop body that contains it. The compiler accepts a `defer` anywhere. staticcheck reports it in a loop that cannot end.

## How to find it

Go to {{file}}:{{line}} and find the resource that the deferred call releases. Choose 1 fix:

- Move the body of the loop into its own function or method. A `defer` in that function runs at the end of each call, so at the end of each iteration.
- Release the resource at the end of the iteration without `defer`. Check that every path through the body does it.

## Example

```go
for {
	job := <-w.jobs
	w.mu.Lock()
	defer w.mu.Unlock() // never runs: the next Lock blocks forever
	job()
}
```

Give each iteration its own function, so that its defer runs:

```go
for {
	w.run(<-w.jobs) // run locks, defers the unlock and calls the job
}
```

## Learn more
