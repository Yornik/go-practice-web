---
id: vet-defers
source: vet
analyzer: defers
match: ^call to time\.Since is not deferred$
title: time.Since runs at the defer statement, not when the function returns
links:
- The Go spec: Defer statements | https://go.dev/ref/spec#Defer_statements
- Effective Go: Defer | https://go.dev/doc/effective_go#defer
- The defers check of go vet | https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/defers
---
## What it means

In `defer log.Printf("took %v", time.Since(start))`, Go evaluates the arguments of the deferred call when the `defer` statement runs. So `time.Since(start)` is computed right away, and it is almost 0. Only the call to `log.Printf` waits until the function returns, and it prints that early value. The duration in the log is wrong for every call.

## Why Go does this

The Go spec says that each time a `defer` statement runs, the function value and the parameters to the call are evaluated and saved. Only the call itself is postponed. That rule makes `defer f.Close()` reliable, because it closes the file that `f` holds at that moment. For a measurement it is a trap, so `go vet` reports `time.Since` as an argument of a deferred call.

## How to find it

Go to {{file}}:{{line}}. Everything that must happen at the end of the function belongs inside a function literal: `defer func() { ... }()`. The code in the literal runs when the function returns, so `time.Since` then measures the whole call. Ask the same question for every deferred call with arguments: are the values correct at the moment of the `defer`?

## Example

```go
start := time.Now()
defer log.Printf("took %v", time.Since(start)) // measured now: about 0
```

Defer a function literal, so the measurement runs at the end:

```go
start := time.Now()
defer func() { log.Printf("took %v", time.Since(start)) }()
```

## Learn more
